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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Kany8sControlPlaneTemplateSpec defines the desired state of Kany8sControlPlaneTemplate.
type Kany8sControlPlaneTemplateSpec struct {
	// template describes the desired state of Kany8sControlPlaneTemplate.
	// +required
	Template Kany8sControlPlaneTemplateResource `json:"template"`
}

// Kany8sControlPlaneTemplateResource describes the data needed to create a
// Kany8sControlPlane from a ClusterClass template.
type Kany8sControlPlaneTemplateResource struct {
	// metadata is the standard object's metadata.
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec is the desired state of Kany8sControlPlaneTemplateResource.
	// +required
	Spec Kany8sControlPlaneTemplateResourceSpec `json:"spec"`
}

// Kany8sControlPlaneTemplateResourceSpec defines the desired state of a
// Kany8sControlPlane created from a template.
//
// NOTE: This spec is similar to Kany8sControlPlaneSpec but omits Version and
// controlPlaneEndpoint. Those fields are controlled by Cluster topology and the
// controller, respectively.
type Kany8sControlPlaneTemplateResourceSpec struct {
	// resourceGraphDefinitionRef selects the kro ResourceGraphDefinition used to
	// provision the managed control plane.
	//
	// When set, this selects the kro backend.
	// +optional
	ResourceGraphDefinitionRef *ResourceGraphDefinitionReference `json:"resourceGraphDefinitionRef,omitempty"`

	// kroSpec is an arbitrary, provider-specific object passed through to the kro
	// instance spec.
	// +optional
	KroSpec *apiextensionsv1.JSON `json:"kroSpec,omitempty"`

	// kubeadm selects the builtin kubeadm backend.
	// +optional
	Kubeadm *Kany8sControlPlaneKubeadmSpec `json:"kubeadm,omitempty"`

	// externalBackend selects an out-of-tree backend.
	// +optional
	ExternalBackend *Kany8sControlPlaneExternalBackendSpec `json:"externalBackend,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1alpha1"

// Kany8sControlPlaneTemplate is the Schema for the kany8scontrolplanetemplates API
type Kany8sControlPlaneTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Kany8sControlPlaneTemplate
	// +required
	Spec Kany8sControlPlaneTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// Kany8sControlPlaneTemplateList contains a list of Kany8sControlPlaneTemplate
type Kany8sControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Kany8sControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kany8sControlPlaneTemplate{}, &Kany8sControlPlaneTemplateList{})
}
