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

package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1alpha1-kany8scontrolplane,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanes,verbs=create;update,versions=v1alpha1,name=vkany8scontrolplane.kb.io,admissionReviewVersions=v1

func (r *Kany8sControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithValidator(&kany8sControlPlaneValidator{}).
		Complete()
}

type kany8sControlPlaneValidator struct{}

func (v *kany8sControlPlaneValidator) ValidateCreate(_ context.Context, obj *Kany8sControlPlane) (admission.Warnings, error) {
	if obj == nil {
		return nil, nil
	}
	return nil, validateKany8sControlPlaneSpecOnCreate(obj)
}

func (v *kany8sControlPlaneValidator) ValidateUpdate(_ context.Context, oldObj *Kany8sControlPlane, newObj *Kany8sControlPlane) (admission.Warnings, error) {
	if oldObj == nil || newObj == nil {
		return nil, nil
	}
	return nil, validateKany8sControlPlaneSpecOnUpdate(oldObj, newObj)
}

func (v *kany8sControlPlaneValidator) ValidateDelete(_ context.Context, _ *Kany8sControlPlane) (admission.Warnings, error) {
	return nil, nil
}

func validateKany8sControlPlaneSpecOnCreate(obj *Kany8sControlPlane) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	_, newCount := selectedKany8sControlPlaneBackend(obj.Spec)
	if newCount != 1 {
		allErrs = append(allErrs, field.Invalid(specPath, backendSelectionSummary(obj.Spec), "exactly one backend must be selected"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Kany8sControlPlane"}, obj.Name, allErrs)
}

func validateKany8sControlPlaneSpecOnUpdate(oldObj *Kany8sControlPlane, newObj *Kany8sControlPlane) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	oldSelection, oldCount := selectedKany8sControlPlaneBackend(oldObj.Spec)
	newSelection, newCount := selectedKany8sControlPlaneBackend(newObj.Spec)
	if newCount != 1 {
		allErrs = append(allErrs, field.Invalid(specPath, backendSelectionSummary(newObj.Spec), "exactly one backend must be selected"))
	}

	// Backend type changes are treated as replace-the-control-plane, not in-place mutation.
	if oldCount == 1 && newCount == 1 && oldSelection != newSelection {
		allErrs = append(allErrs, field.Forbidden(specPath, fmt.Sprintf("backend type is immutable (from %q to %q)", oldSelection, newSelection)))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Kany8sControlPlane"}, newObj.Name, allErrs)
}

type kany8sControlPlaneBackendSelection string

const (
	kany8sControlPlaneBackendSelectionNone     kany8sControlPlaneBackendSelection = "none"
	kany8sControlPlaneBackendSelectionKro      kany8sControlPlaneBackendSelection = "kro"
	kany8sControlPlaneBackendSelectionKubeadm  kany8sControlPlaneBackendSelection = "kubeadm"
	kany8sControlPlaneBackendSelectionExternal kany8sControlPlaneBackendSelection = "external"
)

func selectedKany8sControlPlaneBackend(spec Kany8sControlPlaneSpec) (kany8sControlPlaneBackendSelection, int) {
	count := 0
	selection := kany8sControlPlaneBackendSelectionNone
	if spec.ResourceGraphDefinitionRef != nil {
		count++
		selection = kany8sControlPlaneBackendSelectionKro
	}
	if spec.Kubeadm != nil {
		count++
		selection = kany8sControlPlaneBackendSelectionKubeadm
	}
	if spec.ExternalBackend != nil {
		count++
		selection = kany8sControlPlaneBackendSelectionExternal
	}

	if count != 1 {
		return kany8sControlPlaneBackendSelectionNone, count
	}
	return selection, count
}

func backendSelectionSummary(spec Kany8sControlPlaneSpec) map[string]bool {
	return map[string]bool{
		"resourceGraphDefinitionRef": spec.ResourceGraphDefinitionRef != nil,
		"kubeadm":                    spec.Kubeadm != nil,
		"externalBackend":            spec.ExternalBackend != nil,
	}
}
