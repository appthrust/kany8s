package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/endpoint"
	"github.com/reoring/kany8s/internal/kro"
)

// Kany8sControlPlaneReconciler reconciles a Kany8sControlPlane object.
type Kany8sControlPlaneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kro.run,resources=resourcegraphdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=kro.run,resources=*,verbs=create;get;list;watch;update;patch

func (r *Kany8sControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cp := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, cp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if cp.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	gvk, err := kro.ResolveInstanceGVK(ctx, r.Client, cp.Spec.ResourceGraphDefinitionRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance := &unstructured.Unstructured{Object: map[string]any{}}
	instance.SetGroupVersionKind(gvk)
	instance.SetName(cp.Name)
	instance.SetNamespace(cp.Namespace)

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, instance, func() error {
		if err := controllerutil.SetControllerReference(cp, instance, r.Scheme); err != nil {
			return err
		}

		spec, err := buildKroInstanceSpec(cp.Spec.KroSpec, cp.Spec.Version)
		if err != nil {
			return err
		}
		return unstructured.SetNestedField(instance.Object, spec, "spec")
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("reconciled kro instance", "operation", op, "gvk", gvk.String(), "name", instance.GetName(), "namespace", instance.GetNamespace())

	if err := r.Get(ctx, client.ObjectKeyFromObject(instance), instance); err != nil {
		return ctrl.Result{}, err
	}

	instStatus, err := kro.ReadInstanceStatus(instance)
	if err != nil {
		logger.Error(err, "failed to read kro instance normalized status", "gvk", gvk.String(), "name", instance.GetName(), "namespace", instance.GetNamespace())
	}

	var (
		parsedEndpoint   *clusterv1.APIEndpoint
		endpointParseErr error
	)
	if instStatus.Endpoint != nil {
		ep, err := endpoint.Parse(*instStatus.Endpoint)
		if err != nil {
			endpointParseErr = err
		} else {
			parsedEndpoint = &ep
		}
	}

	if parsedEndpoint != nil && !apiEndpointEqual(cp.Spec.ControlPlaneEndpoint, parsedEndpoint) {
		patch := client.MergeFrom(cp.DeepCopy())
		cp.Spec.ControlPlaneEndpoint = &clusterv1.APIEndpoint{Host: parsedEndpoint.Host, Port: parsedEndpoint.Port}
		if err := r.Patch(ctx, cp, patch); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("updated control plane endpoint", "host", parsedEndpoint.Host, "port", parsedEndpoint.Port)

		if err := r.Get(ctx, req.NamespacedName, cp); err != nil {
			return ctrl.Result{}, err
		}
	}

	statusPatch := client.MergeFrom(cp.DeepCopy())

	if parsedEndpoint != nil {
		if cp.Status.Initialization == nil {
			cp.Status.Initialization = &controlplanev1alpha1.Kany8sControlPlaneInitializationStatus{}
		}
		// Only ever flip to true (first time the endpoint becomes available).
		if !cp.Status.Initialization.ControlPlaneInitialized {
			cp.Status.Initialization.ControlPlaneInitialized = true
		}
	}

	instanceReady := instStatus.Ready != nil && *instStatus.Ready
	controlPlaneReady := instanceReady && parsedEndpoint != nil

	if controlPlaneReady {
		setReadyCondition(cp, corev1.ConditionTrue, "Ready", "", clusterv1.ConditionSeverityNone)
	} else {
		msg := ""
		if instStatus.Message != nil {
			msg = *instStatus.Message
		}
		if msg == "" && endpointParseErr != nil {
			msg = endpointParseErr.Error()
		}
		if msg == "" && instanceReady && parsedEndpoint == nil {
			msg = "waiting for a valid control plane endpoint"
		}
		setReadyCondition(cp, corev1.ConditionFalse, "Creating", msg, clusterv1.ConditionSeverityInfo)
	}

	if controlPlaneReady {
		cp.Status.FailureReason = nil
		cp.Status.FailureMessage = nil
	} else {
		if instStatus.Reason != nil {
			r := capierrors.ClusterStatusError(*instStatus.Reason)
			cp.Status.FailureReason = &r
		} else if endpointParseErr != nil {
			r := capierrors.ClusterStatusError("InvalidEndpoint")
			cp.Status.FailureReason = &r
		} else {
			cp.Status.FailureReason = nil
		}

		if instStatus.Message != nil {
			m := *instStatus.Message
			cp.Status.FailureMessage = &m
		} else if endpointParseErr != nil {
			m := endpointParseErr.Error()
			cp.Status.FailureMessage = &m
		} else {
			cp.Status.FailureMessage = nil
		}
	}

	if err := r.Status().Patch(ctx, cp, statusPatch); err != nil {
		return ctrl.Result{}, err
	}

	if !controlPlaneReady {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Kany8sControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1alpha1.Kany8sControlPlane{}).
		Complete(r)
}

func buildKroInstanceSpec(kroSpec *apiextensionsv1.JSON, version string) (map[string]any, error) {
	spec := map[string]any{}

	if kroSpec != nil {
		raw := bytes.TrimSpace(kroSpec.Raw)
		if len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, fmt.Errorf("unmarshal kroSpec: %w", err)
			}
			if v != nil {
				m, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("kroSpec must be an object, got %T", v)
				}
				spec = m
			}
		}
	}

	spec["version"] = version
	return spec, nil
}

func apiEndpointEqual(a, b *clusterv1.APIEndpoint) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Host == b.Host && a.Port == b.Port
}

func setReadyCondition(cp *controlplanev1alpha1.Kany8sControlPlane, status corev1.ConditionStatus, reason, message string, severity clusterv1.ConditionSeverity) {
	if cp == nil {
		return
	}

	now := metav1.NewTime(time.Now().UTC().Truncate(time.Second))

	cond := clusterv1.Condition{
		Type:               clusterv1.ReadyCondition,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
	if status == corev1.ConditionFalse {
		cond.Severity = severity
	}

	for i := range cp.Status.Conditions {
		existing := cp.Status.Conditions[i]
		if existing.Type != clusterv1.ReadyCondition {
			continue
		}
		if conditionSameState(existing, cond) {
			cond.LastTransitionTime = existing.LastTransitionTime
		}
		cp.Status.Conditions[i] = cond
		sortConditions(cp)
		return
	}

	cp.Status.Conditions = append(cp.Status.Conditions, cond)
	sortConditions(cp)
}

func conditionSameState(a, b clusterv1.Condition) bool {
	return a.Type == b.Type &&
		a.Status == b.Status &&
		a.Reason == b.Reason &&
		a.Severity == b.Severity &&
		a.Message == b.Message
}

func sortConditions(cp *controlplanev1alpha1.Kany8sControlPlane) {
	if cp == nil {
		return
	}
	sort.SliceStable(cp.Status.Conditions, func(i, j int) bool {
		a := cp.Status.Conditions[i]
		b := cp.Status.Conditions[j]
		if a.Type == clusterv1.ReadyCondition {
			return true
		}
		if b.Type == clusterv1.ReadyCondition {
			return false
		}
		return a.Type < b.Type
	})
}
