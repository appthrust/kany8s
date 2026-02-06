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

// +kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1alpha1-kany8scontrolplanetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=kany8scontrolplanetemplates,verbs=create;update,versions=v1alpha1,name=vkany8scontrolplanetemplate.kb.io,admissionReviewVersions=v1

func (r *Kany8sControlPlaneTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithValidator(&kany8sControlPlaneTemplateValidator{}).
		Complete()
}

type kany8sControlPlaneTemplateValidator struct{}

func (v *kany8sControlPlaneTemplateValidator) ValidateCreate(_ context.Context, obj *Kany8sControlPlaneTemplate) (admission.Warnings, error) {
	if obj == nil {
		return nil, nil
	}
	return nil, validateKany8sControlPlaneTemplateSpecOnCreate(obj)
}

func (v *kany8sControlPlaneTemplateValidator) ValidateUpdate(_ context.Context, oldObj *Kany8sControlPlaneTemplate, newObj *Kany8sControlPlaneTemplate) (admission.Warnings, error) {
	if oldObj == nil || newObj == nil {
		return nil, nil
	}
	return nil, validateKany8sControlPlaneTemplateSpecOnUpdate(oldObj, newObj)
}

func (v *kany8sControlPlaneTemplateValidator) ValidateDelete(_ context.Context, _ *Kany8sControlPlaneTemplate) (admission.Warnings, error) {
	return nil, nil
}

func validateKany8sControlPlaneTemplateSpecOnCreate(obj *Kany8sControlPlaneTemplate) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec", "template", "spec")

	_, newCount := selectedKany8sControlPlaneTemplateBackend(obj.Spec.Template.Spec)
	if newCount != 1 {
		allErrs = append(allErrs, field.Invalid(specPath, backendSelectionSummaryForTemplate(obj.Spec.Template.Spec), "exactly one backend must be selected"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Kany8sControlPlaneTemplate"}, obj.Name, allErrs)
}

func validateKany8sControlPlaneTemplateSpecOnUpdate(oldObj *Kany8sControlPlaneTemplate, newObj *Kany8sControlPlaneTemplate) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec", "template", "spec")

	oldSelection, oldCount := selectedKany8sControlPlaneTemplateBackend(oldObj.Spec.Template.Spec)
	newSelection, newCount := selectedKany8sControlPlaneTemplateBackend(newObj.Spec.Template.Spec)
	if newCount != 1 {
		allErrs = append(allErrs, field.Invalid(specPath, backendSelectionSummaryForTemplate(newObj.Spec.Template.Spec), "exactly one backend must be selected"))
	}

	if oldCount == 1 && newCount == 1 && oldSelection != newSelection {
		allErrs = append(allErrs, field.Forbidden(specPath, fmt.Sprintf("backend type is immutable (from %q to %q)", oldSelection, newSelection)))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Kany8sControlPlaneTemplate"}, newObj.Name, allErrs)
}

type kany8sControlPlaneTemplateBackendSelection string

const (
	kany8sControlPlaneTemplateBackendSelectionNone     kany8sControlPlaneTemplateBackendSelection = "none"
	kany8sControlPlaneTemplateBackendSelectionKro      kany8sControlPlaneTemplateBackendSelection = "kro"
	kany8sControlPlaneTemplateBackendSelectionKubeadm  kany8sControlPlaneTemplateBackendSelection = "kubeadm"
	kany8sControlPlaneTemplateBackendSelectionExternal kany8sControlPlaneTemplateBackendSelection = "external"
)

func selectedKany8sControlPlaneTemplateBackend(spec Kany8sControlPlaneTemplateResourceSpec) (kany8sControlPlaneTemplateBackendSelection, int) {
	count := 0
	selection := kany8sControlPlaneTemplateBackendSelectionNone
	if spec.ResourceGraphDefinitionRef != nil {
		count++
		selection = kany8sControlPlaneTemplateBackendSelectionKro
	}
	if spec.Kubeadm != nil {
		count++
		selection = kany8sControlPlaneTemplateBackendSelectionKubeadm
	}
	if spec.ExternalBackend != nil {
		count++
		selection = kany8sControlPlaneTemplateBackendSelectionExternal
	}

	if count != 1 {
		return kany8sControlPlaneTemplateBackendSelectionNone, count
	}
	return selection, count
}

func backendSelectionSummaryForTemplate(spec Kany8sControlPlaneTemplateResourceSpec) map[string]bool {
	return map[string]bool{
		"resourceGraphDefinitionRef": spec.ResourceGraphDefinitionRef != nil,
		"kubeadm":                    spec.Kubeadm != nil,
		"externalBackend":            spec.ExternalBackend != nil,
	}
}
