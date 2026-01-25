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
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	defaultNotReadyMessage = "waiting for control plane to become ready"

	reasonResourceGraphDefinitionNotFound = "ResourceGraphDefinitionNotFound"
	reasonResourceGraphDefinitionInvalid  = "ResourceGraphDefinitionInvalid"

	rgdResolveRequeueAfter = 30 * time.Second
)

// Kany8sControlPlaneReconciler reconciles a Kany8sControlPlane object
type Kany8sControlPlaneReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	InstanceWatcher *dynamicwatch.Watcher
}

// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups=kro.run,resources=resourcegraphdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=kro.run,resources=*,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Kany8sControlPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.0/pkg/reconcile
func (r *Kany8sControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cp := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, cp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	instanceGVK, err := kro.ResolveInstanceGVK(ctx, r, cp.Spec.ResourceGraphDefinitionRef.Name)
	if err != nil {
		log.Error(err, "resolve kro instance GVK")
		return r.requeueWithRGDResolutionCondition(ctx, cp, err)
	}
	if r.InstanceWatcher != nil {
		if err := r.InstanceWatcher.EnsureWatch(ctx, instanceGVK); err != nil {
			log.Error(err, "ensure dynamic watch for kro instance", "gvk", instanceGVK.String())
		}
	}

	instance := &unstructured.Unstructured{}
	instance.SetGroupVersionKind(instanceGVK)
	instance.SetName(cp.Name)
	instance.SetNamespace(cp.Namespace)

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, instance, func() error {
		instance.SetGroupVersionKind(instanceGVK)
		instance.SetName(cp.Name)
		instance.SetNamespace(cp.Namespace)
		if err := controllerutil.SetControllerReference(cp, instance, r.Scheme); err != nil {
			return err
		}

		spec := map[string]any{}
		if cp.Spec.KroSpec != nil && len(cp.Spec.KroSpec.Raw) > 0 {
			if err := json.Unmarshal(cp.Spec.KroSpec.Raw, &spec); err != nil {
				return err
			}
		}
		spec["version"] = cp.Spec.Version
		instance.Object["spec"] = spec

		return nil
	})
	if err != nil {
		log.Error(err, "create or update kro instance")
		return ctrl.Result{}, err
	}

	instanceStatus, err := kro.ReadInstanceStatus(instance)
	if err != nil {
		log.Error(err, "read kro instance status")
		return ctrl.Result{}, err
	}

	if err := r.reconcileKubeconfigSecret(ctx, cp, instance); err != nil {
		log.Error(err, "reconcile kubeconfig secret")
		return ctrl.Result{}, err
	}

	controlPlaneReady := instanceStatus.Ready && instanceStatus.Endpoint != ""
	if instanceStatus.Endpoint != "" {
		cpEndpoint, err := endpoint.Parse(instanceStatus.Endpoint)
		if err != nil {
			log.Error(err, "parse kro instance status endpoint", "endpoint", instanceStatus.Endpoint)
			return ctrl.Result{}, nil
		}

		if cp.Spec.ControlPlaneEndpoint != cpEndpoint {
			before := cp.DeepCopy()
			cp.Spec.ControlPlaneEndpoint = cpEndpoint
			if err := r.Patch(ctx, cp, client.MergeFrom(before)); err != nil {
				log.Error(err, "update control plane endpoint")
				return ctrl.Result{}, err
			}
		}

		if !cp.Status.Initialization.ControlPlaneInitialized {
			before := cp.DeepCopy()
			cp.Status.Initialization.ControlPlaneInitialized = true
			if err := r.Status().Patch(ctx, cp, client.MergeFrom(before)); err != nil {
				log.Error(err, "update control plane initialized")
				return ctrl.Result{}, err
			}
		}
	}

	if err := r.reconcileConditionsAndFailure(ctx, cp, instanceStatus, controlPlaneReady); err != nil {
		log.Error(err, "update control plane conditions")
		return ctrl.Result{}, err
	}
	if !controlPlaneReady {
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func (r *Kany8sControlPlaneReconciler) reconcileKubeconfigSecret(ctx context.Context, cp *controlplanev1alpha1.Kany8sControlPlane, instance *unstructured.Unstructured) error {
	if cp == nil {
		return fmt.Errorf("control plane is nil")
	}
	if instance == nil {
		return fmt.Errorf("kro instance is nil")
	}

	sourceName, found, err := unstructured.NestedString(instance.Object, "status", "kubeconfigSecretRef", "name")
	if err != nil {
		return fmt.Errorf("read status.kubeconfigSecretRef.name: %w", err)
	}
	if !found || sourceName == "" {
		return nil
	}

	sourceNamespace, foundNamespace, err := unstructured.NestedString(instance.Object, "status", "kubeconfigSecretRef", "namespace")
	if err != nil {
		return fmt.Errorf("read status.kubeconfigSecretRef.namespace: %w", err)
	}
	if !foundNamespace || sourceNamespace == "" {
		sourceNamespace = cp.Namespace
	}

	sourceSecret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: sourceName, Namespace: sourceNamespace}, sourceSecret); err != nil {
		return err
	}

	kc, ok := sourceSecret.Data[kubeconfig.DataKey]
	if !ok {
		return fmt.Errorf("source secret %s/%s missing data[%q]", sourceNamespace, sourceName, kubeconfig.DataKey)
	}

	targetName, err := kubeconfig.SecretName(cp.Name)
	if err != nil {
		return err
	}

	target := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: cp.Namespace}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, target, func() error {
		if err := controllerutil.SetControllerReference(cp, target, r.Scheme); err != nil {
			return err
		}
		if target.Labels == nil {
			target.Labels = map[string]string{}
		}
		target.Labels[kubeconfig.ClusterNameLabelKey] = cp.Name
		target.Type = kubeconfig.SecretType
		if target.Data == nil {
			target.Data = map[string][]byte{}
		}
		target.Data[kubeconfig.DataKey] = kc
		return nil
	})
	if err != nil {
		return err
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
	}

	return r.Status().Patch(ctx, cp, client.MergeFrom(before))
}

func (r *Kany8sControlPlaneReconciler) requeueWithRGDResolutionCondition(ctx context.Context, cp *controlplanev1alpha1.Kany8sControlPlane, resolveErr error) (ctrl.Result, error) {
	reason := reasonResourceGraphDefinitionInvalid
	message := resolveErr.Error()
	if errors.IsNotFound(resolveErr) {
		reason = reasonResourceGraphDefinitionNotFound
		message = fmt.Sprintf("ResourceGraphDefinition %q not found", cp.Spec.ResourceGraphDefinitionRef.Name)
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

	watcher := dynamicwatch.New(dynClient, instanceEvents)
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
