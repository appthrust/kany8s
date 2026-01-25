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

// ResourceGraphDefinitionReference identifies a kro ResourceGraphDefinition.
//
// ResourceGraphDefinition is expected to be cluster-scoped.
type ResourceGraphDefinitionReference struct {
	// name is the name of the ResourceGraphDefinition.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// Kany8sControlPlaneSpec defines the desired state of Kany8sControlPlane.
type Kany8sControlPlaneSpec struct {
	// version is the Kubernetes version to use for the control plane.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// resourceGraphDefinitionRef selects the kro ResourceGraphDefinition used to
	// provision the managed control plane.
	ResourceGraphDefinitionRef ResourceGraphDefinitionReference `json:"resourceGraphDefinitionRef"`

	// kroSpec is an arbitrary, provider-specific object passed through to the kro
	// instance spec.
	// +optional
	KroSpec *apiextensionsv1.JSON `json:"kroSpec,omitempty"`

	// controlPlaneEndpoint is the endpoint used by Cluster API to communicate with
	// the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// Kany8sControlPlaneStatus defines the observed state of Kany8sControlPlane.
type Kany8sControlPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Kany8sControlPlane resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Kany8sControlPlane is the Schema for the kany8scontrolplanes API
type Kany8sControlPlane struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Kany8sControlPlane
	// +required
	Spec Kany8sControlPlaneSpec `json:"spec"`

	// status defines the observed state of Kany8sControlPlane
	// +optional
	Status Kany8sControlPlaneStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// Kany8sControlPlaneList contains a list of Kany8sControlPlane
type Kany8sControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Kany8sControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kany8sControlPlane{}, &Kany8sControlPlaneList{})
}
