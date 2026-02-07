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

// Kany8sClusterTemplateSpec defines the desired state of Kany8sClusterTemplate.
type Kany8sClusterTemplateSpec struct {
	// template describes the desired state of Kany8sClusterTemplate.
	// +required
	Template Kany8sClusterTemplateResource `json:"template"`
}

// Kany8sClusterTemplateResource describes the data needed to create a
// Kany8sCluster from a ClusterClass template.
type Kany8sClusterTemplateResource struct {
	// metadata is the standard object's metadata.
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec is the desired state of Kany8sClusterTemplateResource.
	// +required
	Spec Kany8sClusterTemplateResourceSpec `json:"spec"`
}

// Kany8sClusterTemplateResourceSpec defines the desired state of a Kany8sCluster
// created from a template.
type Kany8sClusterTemplateResourceSpec struct {
	// resourceGraphDefinitionRef identifies the kro ResourceGraphDefinition to use
	// for provisioning infrastructure.
	//
	// When unset, the controller runs in stub mode.
	// +optional
	ResourceGraphDefinitionRef *ResourceGraphDefinitionReference `json:"resourceGraphDefinitionRef,omitempty"`

	// kroSpec is an arbitrary, provider-specific object.
	// +optional
	KroSpec *apiextensionsv1.JSON `json:"kroSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1alpha1"

// Kany8sClusterTemplate is the Schema for the kany8sclustertemplates API
type Kany8sClusterTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Kany8sClusterTemplate
	// +required
	Spec Kany8sClusterTemplateSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// Kany8sClusterTemplateList contains a list of Kany8sClusterTemplate
type Kany8sClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Kany8sClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kany8sClusterTemplate{}, &Kany8sClusterTemplateList{})
}
