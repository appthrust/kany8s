package eks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/reoring/kany8s/internal/kubeconfig"
	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	coreDNSRolloutRestartedAtAnnotationKey   = "eks.kany8s.io/karpenter-bootstrapper.coredns-restarted-at"
	karpenterRolloutRestartedAtAnnotationKey = "eks.kany8s.io/karpenter-bootstrapper.karpenter-restarted-at"
)

func (r *EKSKarpenterBootstrapperReconciler) isACKFargateProfileActive(ctx context.Context, namespace, name string) (bool, error) {
	fp := &unstructured.Unstructured{}
	fp.SetGroupVersionKind(ackFargateProfileGVK)
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, fp); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	status, _ := readNestedString(fp.Object, "status", "status")
	return strings.EqualFold(status, "ACTIVE"), nil
}

func (r *EKSKarpenterBootstrapperReconciler) ensureWorkloadRolloutRestarts(
	ctx context.Context,
	owner *clusterv1.Cluster,
	capiClusterName string,
	corednsProfileActive bool,
	karpenterProfileActive bool,
	fluxInstalled bool,
) (needsRequeue bool, _ error) {
	if r == nil || owner == nil {
		return false, nil
	}

	needCoreDNS := corednsProfileActive && !hasNonEmptyAnnotation(owner.Annotations, coreDNSRolloutRestartedAtAnnotationKey)
	needKarpenter := fluxInstalled && karpenterProfileActive && !hasNonEmptyAnnotation(owner.Annotations, karpenterRolloutRestartedAtAnnotationKey)
	if !needCoreDNS && !needKarpenter {
		return false, nil
	}

	kc, rotatorEnabled, ok, err := r.getWorkloadKubeconfigBytes(ctx, owner, capiClusterName)
	if err != nil {
		return true, err
	}
	if !ok {
		// Without kubeconfig we cannot touch the workload cluster.
		// Requeue quickly only when the rotator is enabled and kubeconfig should appear soon.
		return rotatorEnabled, nil
	}

	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kc)
	if err != nil {
		return true, fmt.Errorf("load workload kubeconfig: %w", err)
	}
	restCfg.Timeout = 20 * time.Second
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return true, fmt.Errorf("new workload clientset: %w", err)
	}

	// Use a fresh timestamp per reconcile to avoid patch collisions.
	restartedAt := r.now().Format(time.RFC3339)

	if needCoreDNS {
		if err := rolloutRestartDeployment(ctx, cs, "kube-system", "coredns", restartedAt); err != nil {
			if apierrors.IsNotFound(err) {
				needsRequeue = true
			} else {
				return true, fmt.Errorf("rollout restart kube-system/coredns: %w", err)
			}
		} else {
			patched, err := r.ensureClusterAnnotation(ctx, owner, coreDNSRolloutRestartedAtAnnotationKey, restartedAt)
			if err != nil {
				return true, err
			}
			if patched {
				r.emitEvent(owner, corev1.EventTypeNormal, reasonWorkloadRolloutRestarted, "rolled out restart of kube-system/coredns")
			}
		}
	}

	if needKarpenter {
		if err := rolloutRestartDeployment(ctx, cs, "karpenter", "karpenter", restartedAt); err != nil {
			if apierrors.IsNotFound(err) {
				needsRequeue = true
			} else {
				return true, fmt.Errorf("rollout restart karpenter/karpenter: %w", err)
			}
		} else {
			patched, err := r.ensureClusterAnnotation(ctx, owner, karpenterRolloutRestartedAtAnnotationKey, restartedAt)
			if err != nil {
				return true, err
			}
			if patched {
				r.emitEvent(owner, corev1.EventTypeNormal, reasonWorkloadRolloutRestarted, "rolled out restart of karpenter/karpenter")
			}
		}
	}

	return needsRequeue, nil
}

func rolloutRestartDeployment(ctx context.Context, cs kubernetes.Interface, namespace, name, restartedAt string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	patchObj := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{
						"kubectl.kubernetes.io/restartedAt": restartedAt,
					},
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patchObj)
	if err != nil {
		return fmt.Errorf("marshal rollout restart patch: %w", err)
	}

	_, err = cs.AppsV1().Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	return err
}

func (r *EKSKarpenterBootstrapperReconciler) getWorkloadKubeconfigBytes(ctx context.Context, owner *clusterv1.Cluster, capiClusterName string) ([]byte, bool, bool, error) {
	if r == nil || owner == nil {
		return nil, false, false, nil
	}

	rotatorEnabled := false
	if owner.Annotations != nil {
		rotatorEnabled = strings.EqualFold(strings.TrimSpace(owner.Annotations[coreeks.EnableAnnotationKey]), coreeks.EnableAnnotationValue)
	}

	secretName, err := kubeconfig.SecretName(capiClusterName)
	if err != nil {
		return nil, rotatorEnabled, false, err
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: owner.Namespace, Name: secretName}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, rotatorEnabled, false, nil
		}
		return nil, rotatorEnabled, false, err
	}

	kc := secret.Data[kubeconfig.DataKey]
	if len(kc) == 0 {
		return nil, rotatorEnabled, false, fmt.Errorf("secret %s/%s missing data[%q]", owner.Namespace, secretName, kubeconfig.DataKey)
	}
	return kc, rotatorEnabled, true, nil
}

func (r *EKSKarpenterBootstrapperReconciler) ensureClusterAnnotation(ctx context.Context, cluster *clusterv1.Cluster, key, value string) (bool, error) {
	if r == nil || cluster == nil {
		return false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, fmt.Errorf("annotation key is empty")
	}

	before := cluster.DeepCopy()
	if cluster.Annotations == nil {
		cluster.Annotations = map[string]string{}
	}
	cluster.Annotations[key] = strings.TrimSpace(value)
	if equality.Semantic.DeepEqual(before.Annotations, cluster.Annotations) {
		return false, nil
	}
	if err := r.Patch(ctx, cluster, client.MergeFrom(before)); err != nil {
		return false, err
	}
	return true, nil
}

func hasNonEmptyAnnotation(annotations map[string]string, key string) bool {
	if len(annotations) == 0 {
		return false
	}
	return strings.TrimSpace(annotations[key]) != ""
}
