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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

// Kany8sControlPlaneSpec defines the desired state of Kany8sControlPlane.
type Kany8sControlPlaneSpec struct {
	// Version is the Kubernetes version for the control plane.
	//
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// ResourceGraphDefinitionRef references the kro ResourceGraphDefinition that materializes this control plane.
	ResourceGraphDefinitionRef ResourceGraphDefinitionRef `json:"resourceGraphDefinitionRef"`

	// KroSpec contains arbitrary provider-specific inputs passed to the kro instance.
	//
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	KroSpec *apiextensionsv1.JSON `json:"kroSpec,omitempty"`

	// ControlPlaneEndpoint is set by the controller once the endpoint becomes available.
	//
	// +optional
	ControlPlaneEndpoint *clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// ResourceGraphDefinitionRef describes a reference to a kro ResourceGraphDefinition.
type ResourceGraphDefinitionRef struct {
	// Name is the name of the referenced ResourceGraphDefinition.
	//
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// Kany8sControlPlaneInitializationStatus holds control plane initialization state.
type Kany8sControlPlaneInitializationStatus struct {
	// ControlPlaneInitialized indicates that the control plane has been initialized.
	//
	// +optional
	ControlPlaneInitialized bool `json:"controlPlaneInitialized,omitempty"`
}

// Kany8sControlPlaneStatus defines the observed state of Kany8sControlPlane.
type Kany8sControlPlaneStatus struct {
	// Initialization tracks whether the control plane has been initialized.
	//
	// +optional
	Initialization *Kany8sControlPlaneInitializationStatus `json:"initialization,omitempty"`

	// Conditions defines current service state of the Kany8sControlPlane.
	//
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// FailureReason indicates that there is a fatal problem reconciling the control plane.
	//
	// +optional
	FailureReason *capierrors.ClusterStatusError `json:"failureReason,omitempty"`

	// FailureMessage indicates that there is a fatal problem reconciling the control plane.
	//
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description="Ready condition status"
// +kubebuilder:printcolumn:name="INITIALIZED",type="boolean",JSONPath=".status.initialization.controlPlaneInitialized",description="Control plane initialized"
// +kubebuilder:printcolumn:name="ENDPOINT",type="string",JSONPath=".spec.controlPlaneEndpoint.host",description="Control plane endpoint host"

// Kany8sControlPlane is the Schema for the kany8scontrolplanes API.
type Kany8sControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Kany8sControlPlaneSpec   `json:"spec"`
	Status Kany8sControlPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// Kany8sControlPlaneList contains a list of Kany8sControlPlane.
type Kany8sControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Kany8sControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Kany8sControlPlane{}, &Kany8sControlPlaneList{})
}

// GetConditions returns the set of conditions for this object.
func (r *Kany8sControlPlane) GetConditions() clusterv1.Conditions {
	return r.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (r *Kany8sControlPlane) SetConditions(conditions clusterv1.Conditions) {
	r.Status.Conditions = conditions
}
