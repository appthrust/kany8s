/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/constants"
	"github.com/reoring/kany8s/internal/dynamicwatch"
	"github.com/reoring/kany8s/internal/endpoint"
	"github.com/reoring/kany8s/internal/kro"
	"github.com/reoring/kany8s/internal/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	conditionTypeResourceGraphDefinitionResolved = "ResourceGraphDefinitionResolved"
	conditionTypeCreating                        = "Creating"
	conditionTypeReady                           = "Ready"
	conditionTypeKubeconfigSecretReconciled      = "KubeconfigSecretReconciled"

	defaultNotReadyMessage = "waiting for control plane to become ready"

	reasonResourceGraphDefinitionNotFound   = "ResourceGraphDefinitionNotFound"
	reasonResourceGraphDefinitionInvalid    = "ResourceGraphDefinitionInvalid"
	reasonBackendNotSelected                = "BackendNotSelected"
	reasonMultipleBackendsSelected          = "MultipleBackendsSelected"
	reasonInvalidKroSpec                    = "InvalidKroSpec"
	reasonInvalidExternalBackend            = "InvalidExternalBackend"
	reasonInvalidExternalBackendSpec        = "InvalidExternalBackendSpec"
	reasonInvalidEndpoint                   = "InvalidEndpoint"
	reasonOwnerClusterNotSet                = "OwnerClusterNotSet"
	reasonOwnerClusterNotFound              = "OwnerClusterNotFound"
	reasonOwnerClusterGetFailed             = "OwnerClusterGetFailed"
	reasonKubeconfigSourceSecretNotFound    = "KubeconfigSourceSecretNotFound"
	reasonKubeconfigSourceSecretGetFailed   = "KubeconfigSourceSecretGetFailed"
	reasonKubeconfigSourceSecretDataMissing = "KubeconfigSourceSecretDataMissing"
	reasonKubeconfigSourceSecretCrossNS     = "KubeconfigSourceSecretCrossNamespace"
	reasonInvalidKubeconfig                 = "InvalidKubeconfig"

	rgdResolveRequeueAfter  = 30 * time.Second
	ensureWatchRequeueAfter = 30 * time.Second
)

type controlPlaneBackend string

const (
	controlPlaneBackendNone     controlPlaneBackend = ""
	controlPlaneBackendKro      controlPlaneBackend = "kro"
	controlPlaneBackendKubeadm  controlPlaneBackend = "kubeadm"
	controlPlaneBackendExternal controlPlaneBackend = "external"
)

type kubeadmBackendReconcileResult struct {
	Status         kro.InstanceStatus
	Initialized    bool
	EnsureWatchErr error
}

type externalBackendReconcileResult struct {
	Instance       *unstructured.Unstructured
	Status         kro.InstanceStatus
	EnsureWatchErr error
}

// Kany8sControlPlaneReconciler reconciles a Kany8sControlPlane object
type Kany8sControlPlaneReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	InstanceWatcher dynamicwatch.Ensurer
}

// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8skubeadmcontrolplanes,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=kro.run,resources=resourcegraphdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=kro.run,resources=*,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=create;get;patch;update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// This controller ensures a 1:1 backend object exists for each
// Kany8sControlPlane, injects the desired Kubernetes version into backend spec,
// and projects normalized backend status back into the ControlPlane contract
// (endpoint/initialized/conditions). It also reconciles the CAPI-compatible
// <cluster>-kubeconfig Secret when a provider-specific kubeconfig source Secret
// is referenced via backend status.
// nolint:gocyclo
func (r *Kany8sControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cp := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, cp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	ownerCluster, ownerClusterErr := util.GetOwnerCluster(ctx, r.Client, cp.ObjectMeta)
	if ownerClusterErr != nil {
		if errors.IsNotFound(ownerClusterErr) {
			log.V(1).Info("owner Cluster not found yet")
		} else {
			log.Error(ownerClusterErr, "get owner Cluster")
		}
	}
	if ownerCluster == nil {
		log.V(1).Info("owner Cluster reference not set yet")
	}

	selection, backendCount := selectedControlPlaneBackend(cp.Spec)
	if backendCount == 0 {
		instanceStatus := kro.InstanceStatus{Ready: false, Reason: reasonBackendNotSelected, Message: "exactly one backend must be selected"}
		if err := r.reconcileConditionsAndFailure(ctx, cp, instanceStatus, false); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}
	if backendCount > 1 {
		instanceStatus := kro.InstanceStatus{Ready: false, Reason: reasonMultipleBackendsSelected, Message: "exactly one backend must be selected"}
		if err := r.reconcileConditionsAndFailure(ctx, cp, instanceStatus, false); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	var (
		instance             *unstructured.Unstructured
		instanceStatus       kro.InstanceStatus
		backendInitialized   bool
		initializeOnEndpoint bool
		ensureWatchErr       error
		kubeconfigEnabled    bool
		kubeconfigRes        ctrl.Result
	)

	switch selection {
	case controlPlaneBackendKro:
		rgdName := cp.Spec.ResourceGraphDefinitionRef.Name

		instanceGVK, err := kro.ResolveInstanceGVK(ctx, r, rgdName)
		if err != nil {
			log.Error(err, "resolve kro instance GVK")
			return r.requeueWithRGDResolutionCondition(ctx, cp, rgdName, err)
		}
		if r.InstanceWatcher != nil {
			if err := r.InstanceWatcher.EnsureWatch(ctx, instanceGVK); err != nil {
				ensureWatchErr = err
				log.Error(err, "ensure dynamic watch for kro instance", "gvk", instanceGVK.String())
			}
		}

		instance = &unstructured.Unstructured{}
		instance.SetGroupVersionKind(instanceGVK)
		instance.SetName(cp.Name)
		instance.SetNamespace(cp.Namespace)

		instanceSpec, err := buildKroInstanceSpec(cp)
		if err != nil {
			log.Error(err, "invalid kroSpec")
			return r.reconcileInvalidKroSpec(ctx, cp, err)
		}

		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, instance, func() error {
			instance.SetGroupVersionKind(instanceGVK)
			instance.SetName(cp.Name)
			instance.SetNamespace(cp.Namespace)
			if err := controllerutil.SetControllerReference(cp, instance, r.Scheme); err != nil {
				return err
			}
			setBackendClusterMetadata(instance, ownerCluster)
			instance.Object["spec"] = instanceSpec

			return nil
		})
		if err != nil {
			log.Error(err, "create or update kro backend object")
			return ctrl.Result{}, err
		}

		instanceStatus, err = kro.ReadInstanceStatus(instance)
		if err != nil {
			log.Error(err, "read kro backend status")
			return ctrl.Result{}, err
		}
		kubeconfigEnabled = true
		initializeOnEndpoint = true

	case controlPlaneBackendKubeadm:
		kubeadmRes, err := r.reconcileKubeadmBackend(ctx, cp, ownerCluster)
		if err != nil {
			log.Error(err, "reconcile kubeadm backend")
			return ctrl.Result{}, err
		}
		instanceStatus = kubeadmRes.Status
		backendInitialized = kubeadmRes.Initialized
		ensureWatchErr = kubeadmRes.EnsureWatchErr
		kubeconfigEnabled = false
		initializeOnEndpoint = false

	case controlPlaneBackendExternal:
		externalRes, err := r.reconcileExternalBackend(ctx, cp, ownerCluster)
		if err != nil {
			if invalidSpecErr, ok := err.(*invalidBackendSpecError); ok {
				log.Error(err, "invalid external backend spec")
				return r.reconcileInvalidBackendSpec(ctx, cp, invalidSpecErr.Reason, invalidSpecErr.Message)
			}
			log.Error(err, "reconcile external backend")
			return ctrl.Result{}, err
		}
		instance = externalRes.Instance
		instanceStatus = externalRes.Status
		ensureWatchErr = externalRes.EnsureWatchErr
		kubeconfigEnabled = true
		initializeOnEndpoint = true

	default:
		instanceStatus = kro.InstanceStatus{Ready: false, Reason: reasonBackendNotSelected, Message: "exactly one backend must be selected"}
	}

	if kubeconfigEnabled {
		var err error
		kubeconfigRes, kubeconfigEnabled, err = r.reconcileKubeconfigSecret(ctx, cp, instance, ownerCluster, ownerClusterErr)
		if err != nil {
			log.Error(err, "reconcile kubeconfig secret")
			return ctrl.Result{}, err
		}
	}

	controlPlaneReady := instanceStatus.Ready && instanceStatus.Endpoint != ""
	endpointValid := true
	if instanceStatus.Endpoint != "" {
		cpEndpoint, err := endpoint.Parse(instanceStatus.Endpoint)
		if err != nil {
			log.Error(err, "parse backend status endpoint")
			instanceStatus.Ready = false
			instanceStatus.Reason = reasonInvalidEndpoint
			instanceStatus.Message = fmt.Sprintf("invalid status.endpoint: %v", err)
			controlPlaneReady = false
			endpointValid = false
		} else if cp.Spec.ControlPlaneEndpoint != cpEndpoint {
			before := cp.DeepCopy()
			cp.Spec.ControlPlaneEndpoint = cpEndpoint
			if err := r.Patch(ctx, cp, client.MergeFrom(before)); err != nil {
				log.Error(err, "update control plane endpoint")
				return ctrl.Result{}, err
			}
		}
	}

	if !backendInitialized && initializeOnEndpoint && endpointValid && instanceStatus.Endpoint != "" {
		backendInitialized = true
	}
	if backendInitialized && !cp.Status.Initialization.ControlPlaneInitialized {
		before := cp.DeepCopy()
		cp.Status.Initialization.ControlPlaneInitialized = true
		if err := r.Status().Patch(ctx, cp, client.MergeFrom(before)); err != nil {
			log.Error(err, "update control plane initialized")
			return ctrl.Result{}, err
		}
	}

	overallReady := controlPlaneReady
	if controlPlaneReady && kubeconfigEnabled && kubeconfigRes.RequeueAfter != 0 {
		overallReady = false
		if cond := meta.FindStatusCondition(cp.Status.Conditions, conditionTypeKubeconfigSecretReconciled); cond != nil {
			instanceStatus.Reason = cond.Reason
			instanceStatus.Message = cond.Message
		} else {
			instanceStatus.Reason = reasonKubeconfigSourceSecretNotFound
			instanceStatus.Message = "waiting for kubeconfig secret to be reconciled"
		}
	}

	if err := r.reconcileConditionsAndFailure(ctx, cp, instanceStatus, overallReady); err != nil {
		log.Error(err, "update control plane conditions")
		return ctrl.Result{}, err
	}
	if !controlPlaneReady {
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}
	if kubeconfigRes.RequeueAfter != 0 {
		return kubeconfigRes, nil
	}
	if ensureWatchErr != nil {
		return ctrl.Result{RequeueAfter: ensureWatchRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

type invalidBackendSpecError struct {
	Reason  string
	Message string
}

func (e *invalidBackendSpecError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func selectedControlPlaneBackend(spec controlplanev1alpha1.Kany8sControlPlaneSpec) (controlPlaneBackend, int) {
	selection := controlPlaneBackendNone
	count := 0
	if spec.ResourceGraphDefinitionRef != nil {
		selection = controlPlaneBackendKro
		count++
	}
	if spec.Kubeadm != nil {
		selection = controlPlaneBackendKubeadm
		count++
	}
	if spec.ExternalBackend != nil {
		selection = controlPlaneBackendExternal
		count++
	}
	if count != 1 {
		return controlPlaneBackendNone, count
	}
	return selection, count
}

func (r *Kany8sControlPlaneReconciler) reconcileKubeadmBackend(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sControlPlane,
	ownerCluster *clusterv1.Cluster,
) (kubeadmBackendReconcileResult, error) {
	var result kubeadmBackendReconcileResult
	if cp == nil {
		return result, fmt.Errorf("control plane is nil")
	}
	if cp.Spec.Kubeadm == nil {
		return result, fmt.Errorf("spec.kubeadm is nil")
	}

	backendGVK := controlplanev1alpha1.GroupVersion.WithKind("Kany8sKubeadmControlPlane")
	if r.InstanceWatcher != nil {
		if err := r.InstanceWatcher.EnsureWatch(ctx, backendGVK); err != nil {
			result.EnsureWatchErr = err
		}
	}

	backend := &controlplanev1alpha1.Kany8sKubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cp.Name,
			Namespace: cp.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, backend, func() error {
		if err := controllerutil.SetControllerReference(cp, backend, r.Scheme); err != nil {
			return err
		}
		setBackendClusterMetadata(backend, ownerCluster)

		backend.Spec.Version = cp.Spec.Version
		if cp.Spec.Kubeadm.Replicas != nil {
			replicas := *cp.Spec.Kubeadm.Replicas
			backend.Spec.Replicas = &replicas
		} else {
			backend.Spec.Replicas = nil
		}
		backend.Spec.MachineTemplate = cp.Spec.Kubeadm.MachineTemplate
		if cp.Spec.Kubeadm.KubeadmConfigSpec != nil {
			backend.Spec.KubeadmConfigSpec = cp.Spec.Kubeadm.KubeadmConfigSpec.DeepCopy()
		} else {
			backend.Spec.KubeadmConfigSpec = nil
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	result.Status = kubeadmBackendStatusToInstanceStatus(backend)
	result.Initialized = backend.Status.Initialization.ControlPlaneInitialized
	return result, nil
}

func (r *Kany8sControlPlaneReconciler) reconcileExternalBackend(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sControlPlane,
	ownerCluster *clusterv1.Cluster,
) (externalBackendReconcileResult, error) {
	var result externalBackendReconcileResult
	if cp == nil {
		return result, fmt.Errorf("control plane is nil")
	}
	if cp.Spec.ExternalBackend == nil {
		return result, fmt.Errorf("spec.externalBackend is nil")
	}

	gv, err := schema.ParseGroupVersion(cp.Spec.ExternalBackend.APIVersion)
	if err != nil {
		return result, &invalidBackendSpecError{
			Reason:  reasonInvalidExternalBackend,
			Message: fmt.Sprintf("parse spec.externalBackend.apiVersion: %v", err),
		}
	}
	if cp.Spec.ExternalBackend.Kind == "" {
		return result, &invalidBackendSpecError{
			Reason:  reasonInvalidExternalBackend,
			Message: "spec.externalBackend.kind is required",
		}
	}

	backendGVK := schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: cp.Spec.ExternalBackend.Kind}
	if r.InstanceWatcher != nil {
		if err := r.InstanceWatcher.EnsureWatch(ctx, backendGVK); err != nil {
			result.EnsureWatchErr = err
		}
	}

	backendSpec, err := buildExternalBackendSpec(cp, ownerCluster)
	if err != nil {
		return result, &invalidBackendSpecError{
			Reason:  reasonInvalidExternalBackendSpec,
			Message: err.Error(),
		}
	}

	instance := &unstructured.Unstructured{}
	instance.SetGroupVersionKind(backendGVK)
	instance.SetName(cp.Name)
	instance.SetNamespace(cp.Namespace)

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, instance, func() error {
		instance.SetGroupVersionKind(backendGVK)
		instance.SetName(cp.Name)
		instance.SetNamespace(cp.Namespace)
		if err := controllerutil.SetControllerReference(cp, instance, r.Scheme); err != nil {
			return err
		}
		setBackendClusterMetadata(instance, ownerCluster)
		instance.Object["spec"] = backendSpec
		return nil
	})
	if err != nil {
		return result, err
	}

	instanceStatus, err := kro.ReadInstanceStatus(instance)
	if err != nil {
		return result, fmt.Errorf("read external backend status: %w", err)
	}
	result.Instance = instance
	result.Status = instanceStatus
	return result, nil
}

func setBackendClusterMetadata(obj metav1.Object, ownerCluster *clusterv1.Cluster) {
	if obj == nil || ownerCluster == nil {
		return
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[clusterv1.ClusterNameLabel] = ownerCluster.Name
	obj.SetLabels(labels)
	obj.SetOwnerReferences(ensureClusterOwnerReference(obj.GetOwnerReferences(), ownerCluster))
}

func ensureClusterOwnerReference(ownerRefs []metav1.OwnerReference, ownerCluster *clusterv1.Cluster) []metav1.OwnerReference {
	if ownerCluster == nil {
		return ownerRefs
	}
	clusterRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       ownerCluster.Name,
		UID:        ownerCluster.UID,
	}
	for i := range ownerRefs {
		ref := &ownerRefs[i]
		if ref.APIVersion != clusterRef.APIVersion || ref.Kind != clusterRef.Kind || ref.Name != clusterRef.Name {
			continue
		}
		ref.UID = clusterRef.UID
		ref.Controller = nil
		ref.BlockOwnerDeletion = nil
		return ownerRefs
	}
	return append(ownerRefs, clusterRef)
}

func kubeadmBackendStatusToInstanceStatus(backend *controlplanev1alpha1.Kany8sKubeadmControlPlane) kro.InstanceStatus {
	if backend == nil {
		return kro.InstanceStatus{}
	}

	status := kro.InstanceStatus{
		Endpoint: endpointToStatusString(backend.Spec.ControlPlaneEndpoint),
	}
	if cond := meta.FindStatusCondition(backend.Status.Conditions, conditionTypeReady); cond != nil {
		status.Ready = cond.Status == metav1.ConditionTrue
		status.Reason = cond.Reason
		status.Message = cond.Message
	}
	if backend.Status.FailureReason != nil {
		status.Reason = *backend.Status.FailureReason
		status.Terminal = true
	}
	if backend.Status.FailureMessage != nil {
		status.Message = *backend.Status.FailureMessage
		status.Terminal = true
	}
	return status
}

func endpointToStatusString(ep clusterv1.APIEndpoint) string {
	if ep.Host == "" || ep.Port <= 0 {
		return ""
	}
	return fmt.Sprintf("https://%s:%d", ep.Host, ep.Port)
}

func parseJSONSpecObject(raw []byte, fieldName string) (map[string]any, error) {
	spec := map[string]any{}
	if len(raw) == 0 {
		return spec, nil
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("parse %s: %w", fieldName, err)
	}
	if v == nil {
		return nil, fmt.Errorf("%s must be a JSON object, got null", fieldName)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object, got %T", fieldName, v)
	}
	return obj, nil
}

func buildExternalBackendSpec(cp *controlplanev1alpha1.Kany8sControlPlane, ownerCluster *clusterv1.Cluster) (map[string]any, error) {
	if cp == nil {
		return nil, fmt.Errorf("control plane is nil")
	}
	if cp.Spec.ExternalBackend == nil {
		return nil, fmt.Errorf("spec.externalBackend is nil")
	}

	raw := []byte(nil)
	if cp.Spec.ExternalBackend.Spec != nil {
		raw = cp.Spec.ExternalBackend.Spec.Raw
	}
	spec, err := parseJSONSpecObject(raw, "spec.externalBackend.spec")
	if err != nil {
		return nil, err
	}

	spec["version"] = cp.Spec.Version
	if ownerCluster != nil {
		spec["clusterName"] = ownerCluster.Name
		spec["clusterNamespace"] = ownerCluster.Namespace
		spec["clusterUID"] = string(ownerCluster.UID)
	}
	return spec, nil
}

func (r *Kany8sControlPlaneReconciler) reconcileInvalidBackendSpec(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sControlPlane,
	reason string,
	message string,
) (ctrl.Result, error) {
	prev := meta.FindStatusCondition(cp.Status.Conditions, conditionTypeReady)
	shouldEmit := prev == nil || prev.Status != metav1.ConditionFalse || prev.Reason != reason || prev.Message != message

	instanceStatus := kro.InstanceStatus{Ready: false, Reason: reason, Message: message}
	if err := r.reconcileConditionsAndFailure(ctx, cp, instanceStatus, false); err != nil {
		return ctrl.Result{}, err
	}

	if shouldEmit && r.Recorder != nil {
		r.Recorder.Event(cp, corev1.EventTypeWarning, reason, message)
	}
	return ctrl.Result{}, nil
}

// nolint:gocyclo
func (r *Kany8sControlPlaneReconciler) reconcileKubeconfigSecret(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sControlPlane,
	instance *unstructured.Unstructured,
	ownerCluster *clusterv1.Cluster,
	ownerClusterErr error,
) (ctrl.Result, bool, error) {
	if cp == nil {
		return ctrl.Result{}, false, fmt.Errorf("control plane is nil")
	}
	if instance == nil {
		return ctrl.Result{}, false, fmt.Errorf("backend instance is nil")
	}

	sourceName, found, err := unstructured.NestedString(instance.Object, "status", "kubeconfigSecretRef", "name")
	if err != nil {
		return ctrl.Result{}, false, fmt.Errorf("read status.kubeconfigSecretRef.name: %w", err)
	}
	if !found || sourceName == "" {
		return ctrl.Result{}, false, nil
	}

	if ownerClusterErr != nil {
		reason := reasonOwnerClusterGetFailed
		message := fmt.Sprintf("get owner Cluster: %v", ownerClusterErr)
		eventType := corev1.EventTypeWarning
		if errors.IsNotFound(ownerClusterErr) {
			reason = reasonOwnerClusterNotFound
			message = fmt.Sprintf("owner Cluster not found: %v", ownerClusterErr)
			eventType = corev1.EventTypeNormal
		}
		if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionFalse, reason, message, eventType); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, true, nil
	}
	if ownerCluster == nil {
		message := "waiting for owner Cluster reference to be set"
		if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionFalse, reasonOwnerClusterNotSet, message, corev1.EventTypeNormal); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, true, nil
	}
	if ownerCluster.Namespace != cp.Namespace {
		message := fmt.Sprintf("owner Cluster namespace %q does not match control plane namespace %q", ownerCluster.Namespace, cp.Namespace)
		if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionFalse, reasonOwnerClusterGetFailed, message, corev1.EventTypeWarning); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, true, nil
	}

	sourceNamespace, foundNamespace, err := unstructured.NestedString(instance.Object, "status", "kubeconfigSecretRef", "namespace")
	if err != nil {
		return ctrl.Result{}, true, fmt.Errorf("read status.kubeconfigSecretRef.namespace: %w", err)
	}
	if !foundNamespace || sourceNamespace == "" {
		sourceNamespace = cp.Namespace
	}
	if sourceNamespace != cp.Namespace {
		message := fmt.Sprintf(
			"cross-namespace source secret %s/%s is not allowed (control plane namespace: %s)",
			sourceNamespace,
			sourceName,
			cp.Namespace,
		)
		if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionFalse, reasonKubeconfigSourceSecretCrossNS, message, corev1.EventTypeWarning); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, true, nil
	}

	sourceSecret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: sourceName, Namespace: sourceNamespace}, sourceSecret); err != nil {
		if errors.IsNotFound(err) {
			message := fmt.Sprintf("waiting for source secret %s/%s to be created", sourceNamespace, sourceName)
			if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionFalse, reasonKubeconfigSourceSecretNotFound, message, corev1.EventTypeNormal); err != nil {
				return ctrl.Result{}, true, err
			}
			// Wait for the provider-specific source Secret to be created.
			return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, true, nil
		}

		message := fmt.Sprintf("get source secret %s/%s: %v", sourceNamespace, sourceName, err)
		if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionFalse, reasonKubeconfigSourceSecretGetFailed, message, corev1.EventTypeWarning); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, true, nil
	}

	kc, ok := sourceSecret.Data[kubeconfig.DataKey]
	if !ok {
		message := fmt.Sprintf("source secret %s/%s missing data[%q]", sourceNamespace, sourceName, kubeconfig.DataKey)
		if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionFalse, reasonKubeconfigSourceSecretDataMissing, message, corev1.EventTypeWarning); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, true, nil
	}
	if _, err := clientcmd.Load(kc); err != nil {
		message := fmt.Sprintf("source secret %s/%s contains invalid kubeconfig: %v", sourceNamespace, sourceName, err)
		if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionFalse, reasonInvalidKubeconfig, message, corev1.EventTypeWarning); err != nil {
			return ctrl.Result{}, true, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, true, nil
	}

	targetName, err := kubeconfig.SecretName(ownerCluster.Name)
	if err != nil {
		return ctrl.Result{}, true, err
	}

	target := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: cp.Namespace}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, target, func() error {
		if err := controllerutil.SetControllerReference(cp, target, r.Scheme); err != nil {
			return err
		}
		if target.Labels == nil {
			target.Labels = map[string]string{}
		}
		target.Labels[kubeconfig.ClusterNameLabelKey] = ownerCluster.Name
		target.Type = kubeconfig.SecretType
		if target.Data == nil {
			target.Data = map[string][]byte{}
		}
		target.Data[kubeconfig.DataKey] = kc
		return nil
	})
	if err != nil {
		return ctrl.Result{}, true, err
	}

	if err := r.reconcileKubeconfigSecretCondition(ctx, cp, metav1.ConditionTrue, "Reconciled", "kubeconfig secret reconciled", corev1.EventTypeNormal); err != nil {
		return ctrl.Result{}, true, err
	}

	return ctrl.Result{}, true, nil
}

func (r *Kany8sControlPlaneReconciler) reconcileKubeconfigSecretCondition(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sControlPlane,
	status metav1.ConditionStatus,
	reason string,
	message string,
	eventType string,
) error {
	if cp == nil {
		return fmt.Errorf("control plane is nil")
	}

	cond := metav1.Condition{
		Type:               conditionTypeKubeconfigSecretReconciled,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cp.Generation,
	}

	prev := meta.FindStatusCondition(cp.Status.Conditions, cond.Type)
	shouldEmit := prev == nil || prev.Status != cond.Status || prev.Reason != cond.Reason || prev.Message != cond.Message

	before := cp.DeepCopy()
	meta.SetStatusCondition(&cp.Status.Conditions, cond)
	if err := r.Status().Patch(ctx, cp, client.MergeFrom(before)); err != nil {
		return err
	}

	if shouldEmit && r.Recorder != nil {
		r.Recorder.Event(cp, eventType, reason, message)
	}

	return nil
}

func (r *Kany8sControlPlaneReconciler) reconcileConditionsAndFailure(ctx context.Context, cp *controlplanev1alpha1.Kany8sControlPlane, instanceStatus kro.InstanceStatus, controlPlaneReady bool) error {
	before := cp.DeepCopy()

	if controlPlaneReady {
		reason := instanceStatus.Reason
		message := instanceStatus.Message
		if reason == "" {
			reason = "Ready"
		}
		if message == "" {
			message = "control plane is ready"
		}

		conditions.Set(cp, metav1.Condition{
			Type:    conditionTypeReady,
			Status:  metav1.ConditionTrue,
			Reason:  reason,
			Message: message,
		})
		conditions.Delete(cp, conditionTypeCreating)
		cp.Status.FailureReason = nil
		cp.Status.FailureMessage = nil
		if cp.Spec.Version != "" && cp.Status.Version != cp.Spec.Version {
			cp.Status.Version = cp.Spec.Version
		}
	} else {
		reason := instanceStatus.Reason
		message := instanceStatus.Message
		if reason == "" {
			reason = "Creating"
		}
		if message == "" {
			message = defaultNotReadyMessage
		}

		conditions.Set(cp, metav1.Condition{
			Type:    conditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  reason,
			Message: message,
		})
		conditions.Set(cp, metav1.Condition{
			Type:    conditionTypeCreating,
			Status:  metav1.ConditionTrue,
			Reason:  reason,
			Message: message,
		})

		if instanceStatus.Terminal || isTerminalFailureReason(instanceStatus.Reason) {
			if instanceStatus.Reason != "" {
				reasonCopy := instanceStatus.Reason
				cp.Status.FailureReason = &reasonCopy
			} else {
				cp.Status.FailureReason = nil
			}
			if instanceStatus.Message != "" {
				messageCopy := instanceStatus.Message
				cp.Status.FailureMessage = &messageCopy
			} else {
				cp.Status.FailureMessage = nil
			}
		} else {
			cp.Status.FailureReason = nil
			cp.Status.FailureMessage = nil
		}
	}

	return r.Status().Patch(ctx, cp, client.MergeFrom(before))
}

func isTerminalFailureReason(reason string) bool {
	switch reason {
	case reasonBackendNotSelected, reasonMultipleBackendsSelected, reasonInvalidKroSpec, reasonInvalidExternalBackend, reasonInvalidExternalBackendSpec, reasonInvalidEndpoint:
		return true
	default:
		return false
	}
}

func buildKroInstanceSpec(cp *controlplanev1alpha1.Kany8sControlPlane) (map[string]any, error) {
	if cp == nil {
		return nil, fmt.Errorf("control plane is nil")
	}

	raw := []byte(nil)
	if cp.Spec.KroSpec != nil {
		raw = cp.Spec.KroSpec.Raw
	}
	spec, err := parseJSONSpecObject(raw, "spec.kroSpec")
	if err != nil {
		return nil, err
	}

	spec["version"] = cp.Spec.Version
	return spec, nil
}

func (r *Kany8sControlPlaneReconciler) reconcileInvalidKroSpec(ctx context.Context, cp *controlplanev1alpha1.Kany8sControlPlane, kroSpecErr error) (ctrl.Result, error) {
	message := kroSpecErr.Error()

	prev := meta.FindStatusCondition(cp.Status.Conditions, conditionTypeReady)
	shouldEmit := prev == nil || prev.Status != metav1.ConditionFalse || prev.Reason != reasonInvalidKroSpec || prev.Message != message

	instanceStatus := kro.InstanceStatus{Ready: false, Reason: reasonInvalidKroSpec, Message: message}
	if err := r.reconcileConditionsAndFailure(ctx, cp, instanceStatus, false); err != nil {
		return ctrl.Result{}, err
	}

	if shouldEmit && r.Recorder != nil {
		r.Recorder.Event(cp, corev1.EventTypeWarning, reasonInvalidKroSpec, message)
	}

	return ctrl.Result{}, nil
}

func (r *Kany8sControlPlaneReconciler) requeueWithRGDResolutionCondition(ctx context.Context, cp *controlplanev1alpha1.Kany8sControlPlane, rgdName string, resolveErr error) (ctrl.Result, error) {
	reason := reasonResourceGraphDefinitionInvalid
	message := resolveErr.Error()
	if errors.IsNotFound(resolveErr) {
		reason = reasonResourceGraphDefinitionNotFound
		message = fmt.Sprintf("ResourceGraphDefinition %q not found", rgdName)
	}

	cond := metav1.Condition{
		Type:               conditionTypeResourceGraphDefinitionResolved,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cp.Generation,
	}

	prev := meta.FindStatusCondition(cp.Status.Conditions, cond.Type)
	shouldEmit := prev == nil || prev.Status != cond.Status || prev.Reason != cond.Reason || prev.Message != cond.Message
	meta.SetStatusCondition(&cp.Status.Conditions, cond)

	if err := r.Status().Update(ctx, cp); err != nil {
		return ctrl.Result{}, err
	}

	if shouldEmit && r.Recorder != nil {
		r.Recorder.Event(cp, corev1.EventTypeWarning, reason, message)
	}

	return ctrl.Result{RequeueAfter: rgdResolveRequeueAfter}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Kany8sControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	instanceEvents := make(chan event.GenericEvent, 1024)

	dynClient, err := dynamic.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	httpClient, err := rest.HTTPClientFor(mgr.GetConfig())
	if err != nil {
		return err
	}
	mapper, err := apiutil.NewDynamicRESTMapper(mgr.GetConfig(), httpClient)
	if err != nil {
		return err
	}

	watcher := dynamicwatch.NewWithMapper(dynClient, mapper, instanceEvents)
	if err := mgr.Add(watcher); err != nil {
		return err
	}
	r.InstanceWatcher = watcher

	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1alpha1.Kany8sControlPlane{}).
		WatchesRawSource(source.Channel(instanceEvents, &handler.EnqueueRequestForObject{})).
		Named("kany8scontrolplane").
		Complete(r)
}
