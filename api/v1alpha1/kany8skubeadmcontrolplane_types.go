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

// Kany8sKubeadmControlPlaneStatus defines the observed state of Kany8sKubeadmControlPlane.
type Kany8sKubeadmControlPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Kany8sKubeadmControlPlane resource.
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

func init() {
	SchemeBuilder.Register(&Kany8sKubeadmControlPlane{}, &Kany8sKubeadmControlPlaneList{})
}
