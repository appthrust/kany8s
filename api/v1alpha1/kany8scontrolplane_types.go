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
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
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

// Kany8sControlPlaneKubeadmSpec configures the builtin kubeadm backend.
//
// The facade injects and enforces the Kubernetes version from
// Kany8sControlPlane.spec.version.
type Kany8sControlPlaneKubeadmSpec struct {
	// replicas is the desired number of control plane Machines.
	//
	// MVP default is 1.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// machineTemplate is the template used to create control plane Machines.
	MachineTemplate Kany8sKubeadmControlPlaneMachineTemplate `json:"machineTemplate"`

	// kubeadmConfigSpec is the kubeadm bootstrap configuration for control plane Machines.
	// +optional
	KubeadmConfigSpec *bootstrapv1.KubeadmConfigSpec `json:"kubeadmConfigSpec,omitempty"`
}

// Kany8sControlPlaneExternalBackendSpec selects an out-of-tree backend and
// supplies its spec as an arbitrary JSON object.
//
// The facade injects and enforces `.spec.version` on the backend object.
type Kany8sControlPlaneExternalBackendSpec struct {
	// apiVersion is the backend resource apiVersion.
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`

	// kind is the backend resource kind.
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// spec is an arbitrary, backend-specific object passed through to the backend
	// resource `.spec`.
	// +optional
	Spec *apiextensionsv1.JSON `json:"spec,omitempty"`
}

// Kany8sControlPlaneSpec defines the desired state of Kany8sControlPlane.
type Kany8sControlPlaneSpec struct {
	// version is the Kubernetes version to use for the control plane.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// resourceGraphDefinitionRef selects the kro ResourceGraphDefinition used to
	// provision the managed (kro-backed) control plane.
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

	// controlPlaneEndpoint is the endpoint used by Cluster API to communicate with
	// the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// Kany8sControlPlaneInitializationStatus defines fields related to control
// plane initialization.
type Kany8sControlPlaneInitializationStatus struct {
	// controlPlaneInitialized denotes whether the control plane has completed
	// initialization.
	// +optional
	ControlPlaneInitialized bool `json:"controlPlaneInitialized,omitempty"`
}

// Kany8sControlPlaneStatus defines the observed state of Kany8sControlPlane.
type Kany8sControlPlaneStatus struct {
	// version represents the minimum Kubernetes version for the control plane.
	//
	// This field is required by the Cluster API control plane provider contract and is used by the
	// topology controller to determine provisioning and upgrade state.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version string `json:"version,omitempty"`

	// initialization holds fields related to control plane initialization.
	// +optional
	Initialization Kany8sControlPlaneInitializationStatus `json:"initialization,omitzero"`

	// conditions represent the current state of the Kany8sControlPlane resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// failureReason will be set in the event that there is a terminal problem
	// reconciling the control plane and will contain a succinct reason for the
	// failure.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// failureMessage will be set in the event that there is a terminal problem
	// reconciling the control plane and will contain a more verbose string
	// suitable for logging and human consumption.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1alpha1"
// +kubebuilder:printcolumn:name="INITIALIZED",type=boolean,JSONPath=".status.initialization.controlPlaneInitialized"
// +kubebuilder:printcolumn:name="ENDPOINT",type=string,JSONPath=".spec.controlPlaneEndpoint.host"

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

func (r *Kany8sControlPlane) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Kany8sControlPlane) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Kany8sControlPlane{}, &Kany8sControlPlaneList{})
}
