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
	"encoding/json"
	"fmt"

	infrastructurev1alpha1 "github.com/reoring/kany8s/api/infrastructure/v1alpha1"
	"github.com/reoring/kany8s/internal/constants"
	"github.com/reoring/kany8s/internal/kro"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	reasonWaitingForOwnerCluster          = "WaitingForOwnerCluster"
	reasonResourceGraphDefinitionInvalid  = "ResourceGraphDefinitionInvalid"
	reasonResourceGraphDefinitionNotFound = "ResourceGraphDefinitionNotFound"
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

	provisioned := true
	readyStatus := metav1.ConditionTrue
	reason := "Ready"
	message := "infrastructure is ready"
	var failureReason *string
	var failureMessage *string
	var requeueAfter metav1.Duration

	if kc.Spec.ResourceGraphDefinitionRef != nil {
		provisioned = false
		readyStatus = metav1.ConditionFalse
		reason = "Provisioning"
		message = "waiting for infrastructure to become ready"

		rgdName := kc.Spec.ResourceGraphDefinitionRef.Name
		instanceGVK, err := kro.ResolveInstanceGVK(ctx, r, rgdName)
		if err != nil {
			log.Error(err, "resolve kro instance GVK")
			reason = reasonResourceGraphDefinitionInvalid
			message = err.Error()
			if apierrors.IsNotFound(err) {
				reason = reasonResourceGraphDefinitionNotFound
				message = fmt.Sprintf("ResourceGraphDefinition %q not found", rgdName)
			}
			requeueAfter = metav1.Duration{Duration: constants.InfrastructureNotReadyRequeueAfter}
		} else {
			hasClusterUID, err := kro.SchemaHasSpecField(ctx, r, rgdName, "clusterUID")
			if err != nil {
				log.Error(err, "check ResourceGraphDefinition schema")
				reason = reasonResourceGraphDefinitionInvalid
				message = err.Error()
				requeueAfter = metav1.Duration{Duration: constants.InfrastructureNotReadyRequeueAfter}
			} else {
				if hasClusterUID {
					ownerCluster, err := util.GetOwnerCluster(ctx, r.Client, kc.ObjectMeta)
					if err != nil {
						if apierrors.IsNotFound(err) {
							log.V(1).Info("owner Cluster not found yet")
						} else {
							log.Error(err, "get owner Cluster")
						}
						reason = reasonWaitingForOwnerCluster
						message = fmt.Sprintf("waiting for owner Cluster: %v", err)
						requeueAfter = metav1.Duration{Duration: constants.InfrastructureNotReadyRequeueAfter}
					} else if ownerCluster == nil {
						log.V(1).Info("owner Cluster reference not set yet")
						reason = reasonWaitingForOwnerCluster
						message = "waiting for owner Cluster reference to be set"
						requeueAfter = metav1.Duration{Duration: constants.InfrastructureNotReadyRequeueAfter}
					}
				}
			}

			if requeueAfter.Duration == 0 {
				instance := &unstructured.Unstructured{}
				instance.SetGroupVersionKind(instanceGVK)
				instance.SetName(kc.Name)
				instance.SetNamespace(kc.Namespace)

				instanceSpec, err := buildKroInstanceSpec(kc)
				if err != nil {
					log.Error(err, "invalid kroSpec")
					reason = "InvalidKroSpec"
					message = err.Error()
					reasonCopy := reason
					messageCopy := message
					failureReason = &reasonCopy
					failureMessage = &messageCopy
				} else {
					_, err = controllerutil.CreateOrUpdate(ctx, r.Client, instance, func() error {
						instance.SetGroupVersionKind(instanceGVK)
						instance.SetName(kc.Name)
						instance.SetNamespace(kc.Namespace)
						if err := controllerutil.SetControllerReference(kc, instance, r.Scheme); err != nil {
							return err
						}
						instance.Object["spec"] = instanceSpec
						return nil
					})
					if err != nil {
						log.Error(err, "create or update kro instance")
						return ctrl.Result{}, err
					}

					if err := r.Get(ctx, client.ObjectKey{Name: kc.Name, Namespace: kc.Namespace}, instance); err != nil {
						log.Error(err, "get kro instance")
						return ctrl.Result{}, err
					}
					instanceStatus, err := kro.ReadInstanceStatus(instance)
					if err != nil {
						log.Error(err, "read kro instance status")
						return ctrl.Result{}, err
					}

					provisioned = instanceStatus.Ready
					if provisioned {
						readyStatus = metav1.ConditionTrue
						if instanceStatus.Reason != "" {
							reason = instanceStatus.Reason
						} else {
							reason = "Ready"
						}
						if instanceStatus.Message != "" {
							message = instanceStatus.Message
						} else {
							message = "infrastructure is ready"
						}
					} else {
						readyStatus = metav1.ConditionFalse
						if instanceStatus.Reason != "" {
							reason = instanceStatus.Reason
						}
						if instanceStatus.Message != "" {
							message = instanceStatus.Message
						}
						requeueAfter = metav1.Duration{Duration: constants.InfrastructureNotReadyRequeueAfter}
					}
				}
			}
		}
	}

	before := kc.DeepCopy()
	kc.Status.Initialization.Provisioned = provisioned
	conditions.Set(kc, metav1.Condition{Type: "Ready", Status: readyStatus, Reason: reason, Message: message})
	kc.Status.FailureReason = failureReason
	kc.Status.FailureMessage = failureMessage
	if err := r.Status().Patch(ctx, kc, client.MergeFrom(before)); err != nil {
		log.Error(err, "update Kany8sCluster status")
		return ctrl.Result{}, err
	}

	if requeueAfter.Duration != 0 {
		return ctrl.Result{RequeueAfter: requeueAfter.Duration}, nil
	}
	return ctrl.Result{}, nil
}

func buildKroInstanceSpec(kc *infrastructurev1alpha1.Kany8sCluster) (map[string]any, error) {
	if kc == nil {
		return nil, fmt.Errorf("cluster is nil")
	}

	spec := map[string]any{}
	if kc.Spec.KroSpec != nil && len(kc.Spec.KroSpec.Raw) > 0 {
		var v any
		if err := json.Unmarshal(kc.Spec.KroSpec.Raw, &v); err != nil {
			return nil, fmt.Errorf("parse spec.kroSpec: %w", err)
		}
		if v == nil {
			return nil, fmt.Errorf("spec.kroSpec must be a JSON object, got null")
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("spec.kroSpec must be a JSON object, got %T", v)
		}
		spec = obj
	}

	spec["clusterName"] = kc.Name
	spec["clusterNamespace"] = kc.Namespace
	return spec, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Kany8sClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.Kany8sCluster{}).
		Named("infrastructure-kany8scluster").
		Complete(r)
}
