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

package infrastructure

import (
	"context"

	infrastructurev1alpha1 "github.com/reoring/kany8s/api/infrastructure/v1alpha1"
	"github.com/reoring/kany8s/internal/kro"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Kany8sClusterReconciler reconciles a Kany8sCluster object
type Kany8sClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kany8sclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kany8sclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kany8sclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=kro.run,resources=resourcegraphdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=kro.run,resources=*,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// MVP behavior: Kany8sCluster acts as a stub InfrastructureCluster provider.
// It unblocks the CAPI provisioning flow by setting
// status.initialization.provisioned=true and marking the Ready condition True.
func (r *Kany8sClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	kc := &infrastructurev1alpha1.Kany8sCluster{}
	if err := r.Get(ctx, req.NamespacedName, kc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if kc.Spec.ResourceGraphDefinitionRef != nil {
		instanceGVK, err := kro.ResolveInstanceGVK(ctx, r, kc.Spec.ResourceGraphDefinitionRef.Name)
		if err != nil {
			log.Error(err, "resolve kro instance GVK")
			return ctrl.Result{}, err
		}

		instance := &unstructured.Unstructured{}
		instance.SetGroupVersionKind(instanceGVK)
		instance.SetName(kc.Name)
		instance.SetNamespace(kc.Namespace)

		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, instance, func() error {
			instance.SetGroupVersionKind(instanceGVK)
			instance.SetName(kc.Name)
			instance.SetNamespace(kc.Namespace)
			if err := controllerutil.SetControllerReference(kc, instance, r.Scheme); err != nil {
				return err
			}
			if instance.Object["spec"] == nil {
				instance.Object["spec"] = map[string]any{}
			}
			return nil
		})
		if err != nil {
			log.Error(err, "create or update kro instance")
			return ctrl.Result{}, err
		}
	}

	before := kc.DeepCopy()
	kc.Status.Initialization.Provisioned = true
	conditions.Set(kc, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Ready",
		Message: "infrastructure is ready",
	})
	kc.Status.FailureReason = nil
	kc.Status.FailureMessage = nil
	if err := r.Status().Patch(ctx, kc, client.MergeFrom(before)); err != nil {
		log.Error(err, "update Kany8sCluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Kany8sClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.Kany8sCluster{}).
		Named("infrastructure-kany8scluster").
		Complete(r)
}
