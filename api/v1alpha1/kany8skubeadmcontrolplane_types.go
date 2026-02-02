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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Kany8sKubeadmControlPlaneMachineTemplate defines how Machines should be created.
type Kany8sKubeadmControlPlaneMachineTemplate struct {
	// infrastructureRef is a required reference to a provider-specific machine
	// template, for example DockerMachineTemplate.
	InfrastructureRef clusterv1.ContractVersionedObjectReference `json:"infrastructureRef"`
}

// Kany8sKubeadmControlPlaneSpec defines the desired state of Kany8sKubeadmControlPlane.
type Kany8sKubeadmControlPlaneSpec struct {
	// version is the Kubernetes version to use for the control plane.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

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

	// controlPlaneEndpoint is the endpoint used by Cluster API to communicate with
	// the workload control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// Kany8sKubeadmControlPlaneInitializationStatus defines fields related to control
// plane initialization.
type Kany8sKubeadmControlPlaneInitializationStatus struct {
	// controlPlaneInitialized denotes whether the control plane has completed
	// initialization.
	// +optional
	ControlPlaneInitialized bool `json:"controlPlaneInitialized,omitempty"`
}

// Kany8sKubeadmControlPlaneStatus defines the observed state of Kany8sKubeadmControlPlane.
type Kany8sKubeadmControlPlaneStatus struct {
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
	Initialization Kany8sKubeadmControlPlaneInitializationStatus `json:"initialization,omitzero"`

	// conditions represent the current state of the Kany8sKubeadmControlPlane resource.
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

// Kany8sKubeadmControlPlane is the Schema for the kany8skubeadmcontrolplanes API
type Kany8sKubeadmControlPlane struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Kany8sKubeadmControlPlane
	// +required
	Spec Kany8sKubeadmControlPlaneSpec `json:"spec"`

	// status defines the observed state of Kany8sKubeadmControlPlane
	// +optional
	Status Kany8sKubeadmControlPlaneStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// Kany8sKubeadmControlPlaneList contains a list of Kany8sKubeadmControlPlane
type Kany8sKubeadmControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Kany8sKubeadmControlPlane `json:"items"`
}

func (r *Kany8sKubeadmControlPlane) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Kany8sKubeadmControlPlane) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Kany8sKubeadmControlPlane{}, &Kany8sKubeadmControlPlaneList{})
}
