package eks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	karpenterNamespace      = "karpenter"
	karpenterDeploymentName = "karpenter"
)

var (
	karpenterNodePoolGVR     = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"}
	karpenterNodeClaimGVR    = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodeclaims"}
	karpenterEC2NodeClassGVR = schema.GroupVersionResource{Group: "karpenter.k8s.aws", Version: "v1", Resource: "ec2nodeclasses"}
)

func (r *EKSKarpenterBootstrapperReconciler) stopWorkloadKarpenterProvisioning(ctx context.Context, owner *clusterv1.Cluster, capiClusterName, region, eksClusterName, ackClusterName string) error {
	if r == nil || owner == nil {
		return nil
	}

	restCfg, ok, err := r.getWorkloadRESTConfig(ctx, owner, capiClusterName, region, eksClusterName, ackClusterName)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	restCfg.Timeout = 20 * time.Second

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("new workload clientset: %w", err)
	}

	// Stop Karpenter itself first to minimize the chance of replacement nodes.
	if err := scaleDeployment(ctx, cs, karpenterNamespace, karpenterDeploymentName, 0); err != nil {
		// Ignore NotFound (Karpenter might not be installed yet) and Forbidden (best-effort).
		if !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err) {
			return fmt.Errorf("scale down %s/%s: %w", karpenterNamespace, karpenterDeploymentName, err)
		}
	}

	dc, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("new workload dynamic client: %w", err)
	}

	policy := metav1.DeletePropagationBackground
	delOpts := metav1.DeleteOptions{PropagationPolicy: &policy}

	// Best-effort cleanup of Karpenter CRs to avoid provisioning during deletion.
	for _, gvr := range []schema.GroupVersionResource{karpenterNodePoolGVR, karpenterNodeClaimGVR, karpenterEC2NodeClassGVR} {
		if err := deleteCollection(ctx, dc, gvr, delOpts); err != nil {
			if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func (r *EKSKarpenterBootstrapperReconciler) getWorkloadRESTConfig(ctx context.Context, owner *clusterv1.Cluster, capiClusterName, region, eksClusterName, ackClusterName string) (*rest.Config, bool, error) {
	if r == nil || owner == nil {
		return nil, false, nil
	}

	// First try to use the workload kubeconfig Secret (token/rotated by eks-kubeconfig-rotator or provided by CAPI).
	kc, _, ok, err := r.getWorkloadKubeconfigBytes(ctx, owner, capiClusterName)
	if err != nil {
		return nil, false, err
	}
	if ok {
		restCfg, err := clientcmd.RESTConfigFromKubeConfig(kc)
		if err != nil {
			return nil, false, fmt.Errorf("load workload kubeconfig: %w", err)
		}
		return restCfg, true, nil
	}

	// Fallback: build a short-lived token kubeconfig using the ACK EKS Cluster status.
	ackClusterName = strings.TrimSpace(ackClusterName)
	if ackClusterName == "" {
		return nil, false, nil
	}
	ack, err := r.getACKCluster(ctx, owner.Namespace, ackClusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	endpoint, caData := readACKEndpointAndCA(ack)
	if endpoint == "" || caData == "" {
		return nil, false, nil
	}

	resolvedRegion := strings.TrimSpace(region)
	if resolvedRegion == "" {
		resolvedRegion = resolveRegion(owner, ack)
	}
	if resolvedRegion == "" {
		return nil, false, nil
	}

	resolvedEKSName := strings.TrimSpace(eksClusterName)
	if resolvedEKSName == "" {
		if v, ok := readNestedString(ack.Object, "spec", "name"); ok {
			resolvedEKSName = v
		}
	}
	if resolvedEKSName == "" {
		resolvedEKSName = ackClusterName
	}
	if r.TokenGenerator == nil {
		return nil, false, nil
	}
	token, _, err := r.TokenGenerator.Generate(ctx, resolvedRegion, resolvedEKSName)
	if err != nil {
		recordTokenGenerationFailure(metricControllerBootstrapper)
		return nil, false, fmt.Errorf("generate EKS token: %w", err)
	}
	generatedKC, err := coreeks.BuildTokenKubeconfig(capiClusterName, endpoint, caData, token)
	if err != nil {
		return nil, false, err
	}
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(generatedKC)
	if err != nil {
		return nil, false, fmt.Errorf("load generated workload kubeconfig: %w", err)
	}
	return restCfg, true, nil
}

func scaleDeployment(ctx context.Context, cs kubernetes.Interface, namespace, name string, replicas int32) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	patchObj := map[string]any{
		"spec": map[string]any{
			"replicas": replicas,
		},
	}
	patchBytes, err := json.Marshal(patchObj)
	if err != nil {
		return fmt.Errorf("marshal scale patch: %w", err)
	}

	_, err = cs.AppsV1().Deployments(namespace).Patch(ctx, name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	return err
}

func deleteCollection(ctx context.Context, dc dynamic.Interface, gvr schema.GroupVersionResource, opts metav1.DeleteOptions) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return dc.Resource(gvr).DeleteCollection(ctx, opts, metav1.ListOptions{})
}
