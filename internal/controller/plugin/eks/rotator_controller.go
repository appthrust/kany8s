package eks

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/reoring/kany8s/internal/kubeconfig"
	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type EKSKubeconfigRotatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Recorder       recordEventEmitter
	TokenGenerator coreeks.TokenGenerator
	Policy         coreeks.RequeuePolicy
	Now            func() time.Time
	RESTMapper     meta.RESTMapper
}

type recordEventEmitter interface {
	Event(object runtime.Object, eventtype string, reason string, message string)
}

func (r *EKSKubeconfigRotatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	defer func() {
		if retErr != nil {
			recordReconcileError(metricControllerRotator)
		}
	}()

	capiCluster := &v1beta2.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, capiCluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !isRotatorEnabled(capiCluster) {
		return ctrl.Result{}, nil
	}
	if capiCluster.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	capiClusterName, eksClusterName, ackClusterName := resolveClusterNames(capiCluster)
	log := logf.FromContext(ctx).WithValues(
		"cluster", req.String(),
		"eksClusterName", eksClusterName,
		"ackClusterName", ackClusterName,
	)
	ctx = logf.IntoContext(ctx, log)

	if !r.isAPIAvailable(ackClusterGVK) {
		msg := "ACK EKS API (eks.services.k8s.aws/v1alpha1 Cluster) is not available; cause: required CRD/controller is missing. action: install ACK EKS controller and Cluster CRD"
		r.emitEvent(capiCluster, corev1.EventTypeWarning, reasonPrerequisiteAPI, msg)
		return ctrl.Result{RequeueAfter: r.policy().FailureBackoff}, nil
	}

	ackCluster, err := r.getACKCluster(ctx, capiCluster.Namespace, ackClusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("waiting for ACK EKS Cluster %s/%s", capiCluster.Namespace, ackClusterName)
			r.emitEvent(capiCluster, corev1.EventTypeNormal, reasonACKClusterNotFound, msg)
			return ctrl.Result{RequeueAfter: r.policy().FailureBackoff}, nil
		}
		if isNoMatchError(err) {
			msg := "ACK EKS API (eks.services.k8s.aws/v1alpha1 Cluster) is not available; cause: required CRD/controller is missing. action: install ACK EKS controller and Cluster CRD"
			r.emitEvent(capiCluster, corev1.EventTypeWarning, reasonPrerequisiteAPI, msg)
			return ctrl.Result{RequeueAfter: r.policy().FailureBackoff}, nil
		}
		return ctrl.Result{}, err
	}

	endpoint, caData := readACKEndpointAndCA(ackCluster)
	if endpoint == "" || caData == "" {
		msg := fmt.Sprintf("waiting for ACK status fields endpoint/CA on %s/%s", capiCluster.Namespace, ackClusterName)
		r.emitEvent(capiCluster, corev1.EventTypeNormal, reasonACKClusterNotReady, msg)
		return ctrl.Result{RequeueAfter: r.policy().FailureBackoff}, nil
	}

	region := resolveRegion(capiCluster, ackCluster)
	if region == "" {
		msg := fmt.Sprintf(
			"failed to resolve region (checked %q annotation, ACK status.ackResourceMetadata.region, ACK metadata.annotations[%q])",
			coreeks.RegionAnnotationKey,
			coreeks.ACKRegionMetadataAnnotationKey,
		)
		r.emitEvent(capiCluster, corev1.EventTypeWarning, reasonRegionNotResolved, msg)
		return ctrl.Result{RequeueAfter: r.policy().FailureBackoff}, nil
	}
	log = log.WithValues("region", region)
	ctx = logf.IntoContext(ctx, log)
	if r.TokenGenerator == nil {
		return ctrl.Result{}, fmt.Errorf("token generator is not configured")
	}

	tokenValue, expiration, err := r.TokenGenerator.Generate(ctx, region, eksClusterName)
	if err != nil {
		msg := fmt.Sprintf("failed to generate EKS token: %v", err)
		r.emitEvent(capiCluster, corev1.EventTypeWarning, reasonTokenGenerateError, msg)
		recordTokenGenerationFailure(metricControllerRotator)
		return ctrl.Result{RequeueAfter: r.policy().FailureBackoff}, nil
	}

	probeKubeconfig, err := coreeks.BuildTokenKubeconfig(capiClusterName, endpoint, caData, tokenValue)
	if err != nil {
		return ctrl.Result{}, err
	}
	execKubeconfig, err := coreeks.BuildExecKubeconfig(capiClusterName, eksClusterName, region, endpoint, caData)
	if err != nil {
		return ctrl.Result{}, err
	}

	probeName, err := kubeconfig.SecretName(capiClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	probeAnnotations := map[string]string{
		coreeks.TokenExpirationAnnotationKey: expiration.UTC().Format(time.RFC3339),
		coreeks.RegionAnnotationKey:          region,
		coreeks.EKSClusterNameAnnotationKey:  eksClusterName,
	}
	probeResult, err := r.upsertManagedSecret(
		ctx,
		capiCluster,
		probeName,
		probeKubeconfig,
		probeAnnotations,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !probeResult.Managed {
		msg := fmt.Sprintf("secret %s/%s exists and is not managed by %s", capiCluster.Namespace, probeName, coreeks.ManagedByAnnotationValue)
		r.emitEvent(capiCluster, corev1.EventTypeWarning, reasonSecretOwnership, msg)
		recordOwnershipConflict(metricControllerRotator, "Secret")
		return ctrl.Result{RequeueAfter: r.policy().MaxRefresh}, nil
	}
	if probeResult.TakenOver {
		msg := fmt.Sprintf("took over unmanaged secret %s/%s because %q is enabled", capiCluster.Namespace, probeName, coreeks.AllowUnmanagedTakeoverAnnotationKey)
		r.emitEvent(capiCluster, corev1.EventTypeNormal, reasonSecretTakeover, msg)
	}

	execName := probeName + "-exec"
	execAnnotations := map[string]string{
		coreeks.RegionAnnotationKey:         region,
		coreeks.EKSClusterNameAnnotationKey: eksClusterName,
	}
	execResult, err := r.upsertManagedSecret(
		ctx,
		capiCluster,
		execName,
		execKubeconfig,
		execAnnotations,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !execResult.Managed {
		msg := fmt.Sprintf("secret %s/%s exists and is not managed by %s", capiCluster.Namespace, execName, coreeks.ManagedByAnnotationValue)
		r.emitEvent(capiCluster, corev1.EventTypeWarning, reasonSecretOwnership, msg)
		recordOwnershipConflict(metricControllerRotator, "Secret")
	}
	if execResult.TakenOver {
		msg := fmt.Sprintf("took over unmanaged secret %s/%s because %q is enabled", capiCluster.Namespace, execName, coreeks.AllowUnmanagedTakeoverAnnotationKey)
		r.emitEvent(capiCluster, corev1.EventTypeNormal, reasonSecretTakeover, msg)
	}

	if probeResult.Changed || execResult.Changed {
		r.emitEvent(capiCluster, corev1.EventTypeNormal, reasonSecretSynced, "kubeconfig secrets reconciled")
	}

	requeueAfter := coreeks.ComputeNextRequeue(r.now(), expiration, r.policy())
	log.V(1).Info("reconciled kubeconfig secrets", "phase", "steady-state", "requeueAfter", requeueAfter)
	recordSuccessfulSync(metricControllerRotator, r.now())
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

type upsertSecretResult struct {
	Managed   bool
	Changed   bool
	TakenOver bool
}

func (r *EKSKubeconfigRotatorReconciler) upsertManagedSecret(
	ctx context.Context,
	ownerCluster *v1beta2.Cluster,
	secretName string,
	kubeconfigBytes []byte,
	extraAnnotations map[string]string,
) (upsertSecretResult, error) {
	result := upsertSecretResult{Managed: true}

	existing := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: ownerCluster.Namespace, Name: secretName}, existing)
	if err != nil && !apierrors.IsNotFound(err) {
		return result, err
	}

	if apierrors.IsNotFound(err) {
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ownerCluster.Namespace}}
		mutateManagedSecret(secret, ownerCluster, kubeconfigBytes, extraAnnotations)
		if err := controllerutil.SetOwnerReference(ownerCluster, secret, r.Scheme); err != nil {
			return result, err
		}
		if err := r.Create(ctx, secret); err != nil {
			return result, err
		}
		result.Changed = true
		return result, nil
	}

	if !isManagedByRotator(existing.GetAnnotations()) {
		if !coreeks.IsUnmanagedTakeoverEnabled(ownerCluster.GetAnnotations()) {
			result.Managed = false
			return result, nil
		}
		before := existing.DeepCopy()
		mutateManagedSecret(existing, ownerCluster, kubeconfigBytes, extraAnnotations)
		if err := controllerutil.SetOwnerReference(ownerCluster, existing, r.Scheme); err != nil {
			return result, err
		}
		if !equality.Semantic.DeepEqual(before, existing) {
			if err := r.Update(ctx, existing); err != nil {
				return result, err
			}
			result.Changed = true
		}
		result.TakenOver = true
		return result, nil
	}

	before := existing.DeepCopy()
	mutateManagedSecret(existing, ownerCluster, kubeconfigBytes, extraAnnotations)
	if err := controllerutil.SetOwnerReference(ownerCluster, existing, r.Scheme); err != nil {
		return result, err
	}
	if equality.Semantic.DeepEqual(before, existing) {
		return result, nil
	}

	if err := r.Update(ctx, existing); err != nil {
		return result, err
	}
	result.Changed = true
	return result, nil
}

func mutateManagedSecret(secret *corev1.Secret, ownerCluster *v1beta2.Cluster, kubeconfigBytes []byte, extraAnnotations map[string]string) {
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[kubeconfig.ClusterNameLabelKey] = ownerCluster.Name

	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[coreeks.ManagedByAnnotationKey] = coreeks.ManagedByAnnotationValue
	maps.Copy(secret.Annotations, extraAnnotations)

	secret.Type = kubeconfig.SecretType
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	secret.Data[kubeconfig.DataKey] = kubeconfigBytes
}

func isManagedByRotator(annotations map[string]string) bool {
	if len(annotations) == 0 {
		return false
	}
	return strings.TrimSpace(annotations[coreeks.ManagedByAnnotationKey]) == coreeks.ManagedByAnnotationValue
}

func isRotatorEnabled(cluster *v1beta2.Cluster) bool {
	if cluster == nil {
		return false
	}
	if len(cluster.Annotations) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cluster.Annotations[coreeks.EnableAnnotationKey]), coreeks.EnableAnnotationValue)
}

func resolveClusterNames(cluster *v1beta2.Cluster) (capiClusterName string, eksClusterName string, ackClusterName string) {
	if cluster == nil {
		return "", "", ""
	}

	capiClusterName = cluster.Name
	eksClusterName = defaultEKSClusterName(cluster)
	ackClusterName = eksClusterName
	if cluster.Annotations == nil {
		return capiClusterName, eksClusterName, ackClusterName
	}

	if v := strings.TrimSpace(cluster.Annotations[coreeks.EKSClusterNameAnnotationKey]); v != "" {
		eksClusterName = v
		ackClusterName = v
	}
	if v := strings.TrimSpace(cluster.Annotations[coreeks.ACKClusterNameAnnotationKey]); v != "" {
		ackClusterName = v
	}
	return capiClusterName, eksClusterName, ackClusterName
}

func defaultEKSClusterName(cluster *v1beta2.Cluster) string {
	if cluster == nil {
		return ""
	}

	controlPlaneRef := cluster.Spec.ControlPlaneRef
	if isKany8sControlPlaneRef(controlPlaneRef) {
		if cpName := strings.TrimSpace(controlPlaneRef.Name); cpName != "" {
			return cpName
		}
	}
	return cluster.Name
}

func isKany8sControlPlaneRef(ref v1beta2.ContractVersionedObjectReference) bool {
	return strings.TrimSpace(ref.APIGroup) == kany8sControlPlaneAPIGroup &&
		strings.TrimSpace(ref.Kind) == kany8sControlPlaneKind
}

func resolveRegion(cluster *v1beta2.Cluster, ackCluster *unstructured.Unstructured) string {
	if cluster != nil && cluster.Annotations != nil {
		if v := strings.TrimSpace(cluster.Annotations[coreeks.RegionAnnotationKey]); v != "" {
			return v
		}
	}

	if ackCluster != nil {
		if v, ok := readNestedString(ackCluster.Object, "status", "ackResourceMetadata", "region"); ok {
			return v
		}

		annotations := ackCluster.GetAnnotations()
		if v := strings.TrimSpace(annotations[coreeks.ACKRegionMetadataAnnotationKey]); v != "" {
			return v
		}
	}

	return ""
}

func readACKEndpointAndCA(ackCluster *unstructured.Unstructured) (string, string) {
	if ackCluster == nil {
		return "", ""
	}
	endpoint, _ := readNestedString(ackCluster.Object, "status", "endpoint")
	caData, _ := readNestedString(ackCluster.Object, "status", "certificateAuthority", "data")
	return endpoint, caData
}

func readNestedString(obj map[string]any, fields ...string) (string, bool) {
	v, found, err := unstructured.NestedString(obj, fields...)
	if err != nil || !found {
		return "", false
	}
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func (r *EKSKubeconfigRotatorReconciler) getACKCluster(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	ack := &unstructured.Unstructured{}
	ack.SetGroupVersionKind(ackClusterGVK)
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, ack); err != nil {
		return nil, err
	}
	return ack, nil
}

func (r *EKSKubeconfigRotatorReconciler) mapACKClusterToCAPIClusters(ctx context.Context, obj client.Object) []reconcile.Request {
	namespace := obj.GetNamespace()
	ackName := obj.GetName()
	if strings.TrimSpace(ackName) == "" {
		return nil
	}

	clusters := &v1beta2.ClusterList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingFields{ackClusterNameIndexKey: ackName},
	}
	if err := r.List(ctx, clusters, listOpts...); err != nil {
		log := logf.FromContext(ctx).WithValues("namespace", namespace, "ackClusterName", ackName)
		log.V(1).Info("ACK cluster index lookup failed; falling back to namespace list", "error", err.Error())
		clusters = &v1beta2.ClusterList{}
		if err := r.List(ctx, clusters, client.InNamespace(namespace)); err != nil {
			log.Error(err, "list CAPI clusters for ACK mapping")
			return nil
		}
	}

	requests := []reconcile.Request{}
	seen := map[client.ObjectKey]struct{}{}
	for i := range clusters.Items {
		cluster := &clusters.Items[i]
		if !isRotatorEnabled(cluster) {
			continue
		}
		_, _, resolvedAckName := resolveClusterNames(cluster)
		if resolvedAckName != ackName {
			continue
		}
		key := client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.Name}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		requests = append(requests, reconcile.Request{NamespacedName: key})
	}
	return requests
}

func (r *EKSKubeconfigRotatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.TokenGenerator == nil {
		generator, err := coreeks.NewAWSIAMAuthenticatorTokenGenerator()
		if err != nil {
			return err
		}
		r.TokenGenerator = generator
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Policy == (coreeks.RequeuePolicy{}) {
		r.Policy = coreeks.DefaultRequeuePolicy()
	}
	if r.RESTMapper == nil {
		r.RESTMapper = mgr.GetRESTMapper()
	}
	if err := ensureACKClusterNameIndex(context.Background(), mgr); err != nil {
		return err
	}

	ackCluster := &unstructured.Unstructured{}
	ackCluster.SetGroupVersionKind(ackClusterGVK)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1beta2.Cluster{})
	if r.isAPIAvailable(ackClusterGVK) {
		builder = builder.Watches(ackCluster, handler.EnqueueRequestsFromMapFunc(r.mapACKClusterToCAPIClusters))
	} else {
		logf.Log.WithName("setup").Info(
			"skip ACK watch; API is not available",
			"controller", "eks-kubeconfig-rotator",
			"gvk", ackClusterGVK.String(),
		)
	}

	return builder.Named("eks-kubeconfig-rotator").Complete(r)
}

func (r *EKSKubeconfigRotatorReconciler) emitEvent(cluster *v1beta2.Cluster, eventType, reason, message string) {
	if r == nil || r.Recorder == nil || cluster == nil {
		return
	}
	if !controllerEventState.shouldEmit("eks-kubeconfig-rotator", cluster.Namespace, cluster.Name, eventType, reason, message) {
		return
	}
	r.Recorder.Event(cluster, eventType, reason, message)
}

func (r *EKSKubeconfigRotatorReconciler) policy() coreeks.RequeuePolicy {
	return r.Policy.WithDefaults()
}

func (r *EKSKubeconfigRotatorReconciler) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now().UTC()
}

func (r *EKSKubeconfigRotatorReconciler) isAPIAvailable(gvk schema.GroupVersionKind) bool {
	if r == nil || r.RESTMapper == nil {
		return true
	}
	_, err := r.RESTMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil
}
