package eks

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ocmAWSIRSAEnableLabelKey       = "eks.kany8s.io/ocm-awsirsa"
	ocmAWSIRSAEnableLabelValue     = "enabled"
	ocmHubClusterARNAnnotation     = "eks.kany8s.io/ocm-hub-cluster-arn"
	ocmAWSIRSAManagedByValue       = "eks-ocm-awsirsa-bootstrapper"
	ocmManagedClusterRolePrefix    = "ocm-managed-cluster-"
	ocmHubRolePrefix               = "ocm-hub-"
	ocmManagedClusterAssumeHubName = "ocm-managed-cluster-assume-hub"

	reasonOCMAWSIRSADisabled         = "OCMAWSIRSADisabled"
	reasonOCMAWSIRSAInputMissing     = "OCMAWSIRSAInputMissing"
	reasonOCMAWSIRSAInputInvalid     = "OCMAWSIRSAInputInvalid"
	reasonOCMAWSIRSAReconciled       = "OCMAWSIRSAReconciled"
	reasonOCMAWSIRSAOwnership        = "OCMAWSIRSAOwnershipConflict"
	reasonOCMAWSIRSAPrerequisite     = "OCMAWSIRSAPrerequisiteNotReady"
	reasonOCMAWSIRSAResourceTakeover = "OCMAWSIRSAResourceTakeoverApplied"
)

type EKSOcmAWSIRSABootstrapReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Recorder           recordEventEmitter
	FailureBackoff     time.Duration
	SteadyStateRequeue time.Duration
	RESTMapper         meta.RESTMapper
}

func (r *EKSOcmAWSIRSABootstrapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cluster := &clusterv1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if cluster.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	if !isOCMAWSIRSAEnabled(cluster) {
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonOCMAWSIRSADisabled, "OCM AWS IRSA bootstrap is disabled")
		return ctrl.Result{}, nil
	}
	if err := r.ensureClusterNameLabel(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	missing := r.missingAPIs(ackClusterGVK, ackIAMRoleGVK)
	if len(missing) > 0 {
		msg := fmt.Sprintf("missing prerequisite APIs %s; cause: OCM AWS IRSA bootstrap role needs ACK EKS/IAM CRDs. action: install ACK EKS and IAM controllers", joinGVKs(missing))
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonPrerequisiteAPI, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	_, eksClusterName, ackClusterName := resolveClusterNames(cluster)
	log := logf.FromContext(ctx).WithValues(
		"cluster", req.String(),
		"eksClusterName", eksClusterName,
		"ackClusterName", ackClusterName,
	)
	ctx = logf.IntoContext(ctx, log)

	ackCluster, err := r.getACKCluster(ctx, cluster.Namespace, ackClusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("waiting for ACK EKS Cluster %s/%s", cluster.Namespace, ackClusterName)
			r.emitEvent(cluster, corev1.EventTypeNormal, reasonACKClusterNotFound, msg)
			return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
		}
		return ctrl.Result{}, err
	}

	issuerURL, _ := readNestedString(ackCluster.Object, "status", "identity", "oidc", "issuer")
	managedAccountID, _ := readNestedString(ackCluster.Object, "status", "ackResourceMetadata", "ownerAccountID")
	managedClusterARN, _ := readNestedString(ackCluster.Object, "status", "ackResourceMetadata", "arn")
	clusterStatus, _ := readNestedString(ackCluster.Object, "status", "status")
	if issuerURL == "" || managedAccountID == "" || managedClusterARN == "" {
		msg := fmt.Sprintf("waiting for ACK EKS status fields identity.oidc.issuer/ownerAccountID/arn on %s/%s", cluster.Namespace, ackClusterName)
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonACKClusterNotReady, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	if !strings.EqualFold(clusterStatus, "ACTIVE") {
		msg := fmt.Sprintf("waiting for ACK EKS Cluster status.status to become ACTIVE (current=%q) on %s/%s", clusterStatus, cluster.Namespace, ackClusterName)
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonACKClusterNotReady, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	hubClusterARN := strings.TrimSpace(cluster.Annotations[ocmHubClusterARNAnnotation])
	if hubClusterARN == "" {
		msg := fmt.Sprintf("missing %q annotation; cause: OCM AWS IRSA role suffix and assume-hub policy need the hub EKS cluster ARN. action: set %s=arn:aws:eks:<region>:<account>:cluster/<hub>", ocmHubClusterARNAnnotation, ocmHubClusterARNAnnotation)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonOCMAWSIRSAInputMissing, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	hub, err := parseEKSClusterARN(hubClusterARN)
	if err != nil {
		msg := fmt.Sprintf("invalid %q annotation: %v", ocmHubClusterARNAnnotation, err)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonOCMAWSIRSAInputInvalid, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	managed, err := parseEKSClusterARN(managedClusterARN)
	if err != nil {
		msg := fmt.Sprintf("invalid managed cluster ARN from ACK status: %v", err)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonOCMAWSIRSAInputInvalid, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	if managed.AccountID != managedAccountID {
		msg := fmt.Sprintf("ACK EKS ownerAccountID %q does not match managed cluster ARN account %q", managedAccountID, managed.AccountID)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonOCMAWSIRSAInputInvalid, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	issuerHostPath, err := normalizeIssuerHostPath(issuerURL)
	if err != nil {
		msg := fmt.Sprintf("invalid OIDC issuer URL from ACK status: %v", err)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonOCMAWSIRSAInputInvalid, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	oidcProviderARN := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", managed.AccountID, issuerHostPath)
	suffix := ocmAWSIRSASuffix(hub.AccountID, hub.ClusterName, managed.AccountID, managed.ClusterName)
	roleName := ocmManagedClusterRolePrefix + suffix
	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", managed.AccountID, roleName)
	hubRoleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s%s", hub.AccountID, ocmHubRolePrefix, suffix)

	assumePolicy, err := buildOCMManagedClusterAssumeRolePolicyDocument(oidcProviderARN, issuerHostPath)
	if err != nil {
		return ctrl.Result{}, err
	}
	inlinePolicy, err := buildOCMManagedClusterAssumeHubPolicyDocument(hubRoleARN)
	if err != nil {
		return ctrl.Result{}, err
	}
	tags := ocmAWSIRSAResourceTags(cluster, hub, managed)
	if ok, err := r.ensureOCMManagedClusterRole(ctx, cluster, roleName, hub.Region, roleName, assumePolicy, map[string]string{ocmManagedClusterAssumeHubName: inlinePolicy}, tags); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM Role %s/%s exists and is not managed by %s", cluster.Namespace, roleName, ocmAWSIRSAManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonOCMAWSIRSAOwnership, msg)
		recordOwnershipConflict(metricControllerOCMAWSIRSA, "Role")
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	log.V(1).Info("reconciled OCM AWS IRSA bootstrap role", "roleARN", roleARN, "hubRoleARN", hubRoleARN)
	msg := fmt.Sprintf("OCM AWS IRSA managed-cluster role reconciled: %s", roleARN)
	r.emitEvent(cluster, corev1.EventTypeNormal, reasonOCMAWSIRSAReconciled, msg)
	recordSuccessfulSync(metricControllerOCMAWSIRSA, time.Now())
	return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
}

func (r *EKSOcmAWSIRSABootstrapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.FailureBackoff == 0 {
		r.FailureBackoff = 30 * time.Second
	}
	if r.SteadyStateRequeue == 0 {
		r.SteadyStateRequeue = 10 * time.Minute
	}
	if r.RESTMapper == nil {
		r.RESTMapper = mgr.GetRESTMapper()
	}
	if err := ensureACKClusterNameIndex(context.Background(), mgr); err != nil {
		return err
	}

	ackCluster := &unstructured.Unstructured{}
	ackCluster.SetGroupVersionKind(ackClusterGVK)

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(&clusterv1.Cluster{})
	if r.isAPIAvailable(ackClusterGVK) {
		controllerBuilder = controllerBuilder.Watches(ackCluster, handler.EnqueueRequestsFromMapFunc(r.mapACKClusterToCAPIClustersForOCM))
	} else {
		logf.Log.WithName("setup").Info(
			"skip ACK watch; API is not available",
			"controller", ocmAWSIRSAManagedByValue,
			"gvk", ackClusterGVK.String(),
		)
	}
	controllerBuilder = r.withOptionalWatch(controllerBuilder, ackIAMRoleGVK, r.mapManagedObjectToCAPICluster)

	return controllerBuilder.
		Named("eks-ocm-awsirsa-bootstrapper").
		Complete(r)
}

func (r *EKSOcmAWSIRSABootstrapReconciler) ensureClusterNameLabel(ctx context.Context, cluster *clusterv1.Cluster) error {
	if r == nil || cluster == nil {
		return nil
	}
	if cluster.Labels != nil {
		if v := strings.TrimSpace(cluster.Labels[capiClusterNameLabelKey]); v == cluster.Name {
			return nil
		}
	}
	before := cluster.DeepCopy()
	if cluster.Labels == nil {
		cluster.Labels = map[string]string{}
	}
	cluster.Labels[capiClusterNameLabelKey] = cluster.Name
	if equality.Semantic.DeepEqual(before.Labels, cluster.Labels) {
		return nil
	}
	return r.Patch(ctx, cluster, client.MergeFrom(before))
}

func (r *EKSOcmAWSIRSABootstrapReconciler) ensureOCMManagedClusterRole(
	ctx context.Context,
	owner *clusterv1.Cluster,
	name,
	region,
	awsRoleName,
	assumeRolePolicyDocument string,
	inlinePolicies map[string]string,
	tags map[string]string,
) (bool, error) {
	obj := newUnstructured(ackIAMRoleGVK, owner.Namespace, name)
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedByOCMAWSIRSA(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, awsRoleName, "spec", "name")
		mustSetNestedString(u, assumeRolePolicyDocument, "spec", "assumeRolePolicyDocument")
		if len(inlinePolicies) > 0 {
			inlineMap := make(map[string]any, len(inlinePolicies))
			for k, v := range inlinePolicies {
				inlineMap[k] = v
			}
			mustSetNestedField(u, inlineMap, "spec", "inlinePolicies")
		}
		mustSetNestedSlice(u, awsTagsAsSlice(tags), "spec", "tags")
		return nil
	})
}

func (r *EKSOcmAWSIRSABootstrapReconciler) upsertManagedUnstructured(ctx context.Context, owner *clusterv1.Cluster, obj *unstructured.Unstructured, mutate func(*unstructured.Unstructured) error) (bool, error) {
	if owner == nil {
		return false, fmt.Errorf("owner cluster is nil")
	}
	if obj == nil {
		return false, fmt.Errorf("object is nil")
	}
	key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	if err := r.Get(ctx, key, existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, err
		}
		if err := mutate(obj); err != nil {
			return false, err
		}
		if err := popMutationError(obj); err != nil {
			return false, err
		}
		if err := controllerutil.SetOwnerReference(owner, obj, r.Scheme); err != nil {
			return false, err
		}
		if err := r.Create(ctx, obj); err != nil {
			return false, err
		}
		return true, nil
	}

	if !isManagedByOCMAWSIRSA(existing.GetAnnotations()) {
		if !coreeks.IsUnmanagedTakeoverEnabled(owner.GetAnnotations()) {
			return false, nil
		}
		before := existing.DeepCopy()
		if err := mutate(existing); err != nil {
			return false, err
		}
		if err := popMutationError(existing); err != nil {
			return false, err
		}
		if err := controllerutil.SetOwnerReference(owner, existing, r.Scheme); err != nil {
			return false, err
		}
		if !equality.Semantic.DeepEqual(before, existing) {
			if err := r.Update(ctx, existing); err != nil {
				return false, err
			}
		}
		msg := fmt.Sprintf(
			"took over unmanaged %s %s/%s because %q is enabled",
			existing.GroupVersionKind().Kind,
			existing.GetNamespace(),
			existing.GetName(),
			coreeks.AllowUnmanagedTakeoverAnnotationKey,
		)
		r.emitEvent(owner, corev1.EventTypeNormal, reasonOCMAWSIRSAResourceTakeover, msg)
		return true, nil
	}

	before := existing.DeepCopy()
	if err := mutate(existing); err != nil {
		return false, err
	}
	if err := popMutationError(existing); err != nil {
		return false, err
	}
	if err := controllerutil.SetOwnerReference(owner, existing, r.Scheme); err != nil {
		return false, err
	}
	if equality.Semantic.DeepEqual(before, existing) {
		return true, nil
	}
	if err := r.Update(ctx, existing); err != nil {
		return false, err
	}
	return true, nil
}

func (r *EKSOcmAWSIRSABootstrapReconciler) getACKCluster(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ackClusterGVK)
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (r *EKSOcmAWSIRSABootstrapReconciler) mapACKClusterToCAPIClustersForOCM(ctx context.Context, obj client.Object) []reconcile.Request {
	namespace := obj.GetNamespace()
	ackName := obj.GetName()
	if strings.TrimSpace(ackName) == "" {
		return nil
	}

	clusters := &clusterv1.ClusterList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingFields{ackClusterNameIndexKey: ackName},
	}
	if err := r.List(ctx, clusters, listOpts...); err != nil {
		log := logf.FromContext(ctx).WithValues("namespace", namespace, "ackClusterName", ackName)
		log.V(1).Info("ACK cluster index lookup failed; falling back to namespace list", "error", err.Error())
		clusters = &clusterv1.ClusterList{}
		if err := r.List(ctx, clusters, client.InNamespace(namespace)); err != nil {
			log.Error(err, "list CAPI clusters for ACK mapping")
			return nil
		}
	}

	requests := []reconcile.Request{}
	seen := map[client.ObjectKey]struct{}{}
	for i := range clusters.Items {
		cluster := &clusters.Items[i]
		if !isOCMAWSIRSAEnabled(cluster) {
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

func (r *EKSOcmAWSIRSABootstrapReconciler) mapManagedObjectToCAPICluster(_ context.Context, obj client.Object) []reconcile.Request {
	if obj == nil {
		return nil
	}
	labels := obj.GetLabels()
	if len(labels) == 0 {
		return nil
	}
	clusterName := strings.TrimSpace(labels[capiClusterNameLabelKey])
	if clusterName == "" {
		return nil
	}
	if !isManagedByOCMAWSIRSA(obj.GetAnnotations()) {
		return nil
	}
	namespace := strings.TrimSpace(obj.GetNamespace())
	if namespace == "" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: namespace,
			Name:      clusterName,
		},
	}}
}

func (r *EKSOcmAWSIRSABootstrapReconciler) withOptionalWatch(
	b *builder.Builder,
	gvk schema.GroupVersionKind,
	mapFn handler.MapFunc,
) *builder.Builder {
	if !r.isAPIAvailable(gvk) {
		logf.Log.WithName("setup").Info(
			"skip optional watch; API is not available",
			"controller", ocmAWSIRSAManagedByValue,
			"gvk", gvk.String(),
		)
		return b
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	return b.Watches(obj, handler.EnqueueRequestsFromMapFunc(mapFn))
}

func (r *EKSOcmAWSIRSABootstrapReconciler) missingAPIs(gvks ...schema.GroupVersionKind) []schema.GroupVersionKind {
	missing := []schema.GroupVersionKind{}
	for _, gvk := range gvks {
		if r.isAPIAvailable(gvk) {
			continue
		}
		missing = append(missing, gvk)
	}
	return missing
}

func (r *EKSOcmAWSIRSABootstrapReconciler) isAPIAvailable(gvk schema.GroupVersionKind) bool {
	if r == nil || r.RESTMapper == nil {
		return false
	}
	_, err := r.RESTMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil
}

func (r *EKSOcmAWSIRSABootstrapReconciler) emitEvent(cluster *clusterv1.Cluster, eventType, reason, message string) {
	if r == nil || r.Recorder == nil || cluster == nil {
		return
	}
	if !controllerEventState.shouldEmit(metricControllerOCMAWSIRSA, cluster.Namespace, cluster.Name, eventType, reason, message) {
		return
	}
	r.Recorder.Event(cluster, eventType, reason, message)
}

func (r *EKSOcmAWSIRSABootstrapReconciler) failureBackoff() time.Duration {
	if r == nil || r.FailureBackoff == 0 {
		return 30 * time.Second
	}
	return r.FailureBackoff
}

func (r *EKSOcmAWSIRSABootstrapReconciler) steadyStateRequeue() time.Duration {
	if r == nil || r.SteadyStateRequeue == 0 {
		return 10 * time.Minute
	}
	return r.SteadyStateRequeue
}

func isOCMAWSIRSAEnabled(cluster *clusterv1.Cluster) bool {
	if cluster == nil || cluster.Labels == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cluster.Labels[ocmAWSIRSAEnableLabelKey]), ocmAWSIRSAEnableLabelValue)
}

func isManagedByOCMAWSIRSA(annotations map[string]string) bool {
	if len(annotations) == 0 {
		return false
	}
	return strings.TrimSpace(annotations[coreeks.ManagedByAnnotationKey]) == ocmAWSIRSAManagedByValue
}

func setManagedByOCMAWSIRSA(u *unstructured.Unstructured) {
	if u == nil {
		return
	}
	ann := u.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[coreeks.ManagedByAnnotationKey] = ocmAWSIRSAManagedByValue
	u.SetAnnotations(ann)
}

type eksClusterARNParts struct {
	Region      string
	AccountID   string
	ClusterName string
}

func parseEKSClusterARN(raw string) (eksClusterARNParts, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ":")
	if len(parts) != 6 {
		return eksClusterARNParts{}, fmt.Errorf("expected 6 ARN fields, got %d", len(parts))
	}
	if parts[0] != "arn" || parts[2] != "eks" {
		return eksClusterARNParts{}, fmt.Errorf("expected EKS ARN, got %q", raw)
	}
	if strings.TrimSpace(parts[3]) == "" || strings.TrimSpace(parts[4]) == "" {
		return eksClusterARNParts{}, fmt.Errorf("region/account must be non-empty")
	}
	resource := strings.TrimSpace(parts[5])
	const prefix = "cluster/"
	if !strings.HasPrefix(resource, prefix) {
		return eksClusterARNParts{}, fmt.Errorf("expected resource %q prefix", prefix)
	}
	clusterName := strings.TrimSpace(strings.TrimPrefix(resource, prefix))
	if clusterName == "" {
		return eksClusterARNParts{}, fmt.Errorf("cluster name is empty")
	}
	return eksClusterARNParts{Region: parts[3], AccountID: parts[4], ClusterName: clusterName}, nil
}

func ocmAWSIRSASuffix(hubAccountID, hubClusterName, managedAccountID, managedClusterName string) string {
	sum := md5.Sum([]byte(strings.Join([]string{hubAccountID, hubClusterName, managedAccountID, managedClusterName}, "#")))
	return hex.EncodeToString(sum[:])
}

func ocmAWSIRSAResourceTags(owner *clusterv1.Cluster, hub, managed eksClusterARNParts) map[string]string {
	tags := bootstrapperResourceTags(owner, map[string]string{
		"kany8s.io/managed-by":       ocmAWSIRSAManagedByValue,
		"hub_cluster_account_id":     hub.AccountID,
		"hub_cluster_name":           hub.ClusterName,
		"managed_cluster_account_id": managed.AccountID,
		"managed_cluster_name":       managed.ClusterName,
	})
	return tags
}

func buildOCMManagedClusterAssumeRolePolicyDocument(oidcProviderARN, issuerHostPath string) (string, error) {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Effect":    "Allow",
				"Principal": map[string]any{"Federated": oidcProviderARN},
				"Action":    "sts:AssumeRoleWithWebIdentity",
				"Condition": map[string]any{
					"StringEquals": map[string]any{
						issuerHostPath + ":aud": "sts.amazonaws.com",
						issuerHostPath + ":sub": []string{
							"system:serviceaccount:open-cluster-management-agent:klusterlet-registration-sa",
							"system:serviceaccount:open-cluster-management-agent:klusterlet-work-sa",
						},
					},
				},
			},
		},
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal OCM managed-cluster trust policy: %w", err)
	}
	return string(out), nil
}

func buildOCMManagedClusterAssumeHubPolicyDocument(hubRoleARN string) (string, error) {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Effect":   "Allow",
				"Action":   "sts:AssumeRole",
				"Resource": []string{hubRoleARN},
			},
		},
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal OCM managed-cluster assume-hub policy: %w", err)
	}
	return string(out), nil
}
