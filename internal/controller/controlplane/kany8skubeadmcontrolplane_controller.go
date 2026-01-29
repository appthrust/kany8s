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

package controlplane

import (
	"context"
	"fmt"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/constants"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	conditionTypeOwnerClusterResolved = "OwnerClusterResolved"

	reasonOwnerClusterNotSet    = "OwnerClusterNotSet"
	reasonOwnerClusterNotFound  = "OwnerClusterNotFound"
	reasonOwnerClusterResolved  = "OwnerClusterResolved"
	reasonOwnerClusterGetFailed = "OwnerClusterGetFailed"
)

// Kany8sKubeadmControlPlaneReconciler reconciles a Kany8sKubeadmControlPlane object
type Kany8sKubeadmControlPlaneReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8skubeadmcontrolplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8skubeadmcontrolplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8skubeadmcontrolplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// MVP behavior (self-managed bootstrap): resolve the owner Cluster (via
// OwnerReferences) and surface resolution state via Conditions/Events.
func (r *Kany8sKubeadmControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, cp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	owner, err := util.GetOwnerCluster(ctx, r.Client, cp.ObjectMeta)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("owner Cluster not found yet")
			if err := r.reconcileOwnerClusterResolvedCondition(ctx, cp, metav1.ConditionFalse, reasonOwnerClusterNotFound, err.Error(), corev1.EventTypeNormal); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
		}

		log.Error(err, "get owner Cluster")
		message := fmt.Sprintf("get owner Cluster: %v", err)
		if err := r.reconcileOwnerClusterResolvedCondition(ctx, cp, metav1.ConditionFalse, reasonOwnerClusterGetFailed, message, corev1.EventTypeWarning); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}
	if owner == nil {
		message := "waiting for owner Cluster reference to be set"
		if err := r.reconcileOwnerClusterResolvedCondition(ctx, cp, metav1.ConditionFalse, reasonOwnerClusterNotSet, message, corev1.EventTypeNormal); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	if err := r.reconcileOwnerClusterResolvedCondition(ctx, cp, metav1.ConditionTrue, reasonOwnerClusterResolved, "owner Cluster resolved", corev1.EventTypeNormal); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileOwnerClusterResolvedCondition(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sKubeadmControlPlane,
	status metav1.ConditionStatus,
	reason string,
	message string,
	eventType string,
) error {
	if cp == nil {
		return fmt.Errorf("control plane is nil")
	}

	cond := metav1.Condition{
		Type:               conditionTypeOwnerClusterResolved,
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

// SetupWithManager sets up the controller with the Manager.
func (r *Kany8sKubeadmControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1alpha1.Kany8sKubeadmControlPlane{}).
		Named("controlplane-kany8skubeadmcontrolplane").
		Complete(r)
}
