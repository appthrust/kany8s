package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/kro"
)

// Kany8sControlPlaneReconciler reconciles a Kany8sControlPlane object.
type Kany8sControlPlaneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups=kro.run,resources=resourcegraphdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=kro.run,resources=*,verbs=get;create;update

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
