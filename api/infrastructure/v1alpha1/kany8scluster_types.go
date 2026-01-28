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
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Kany8sClusterSpec defines the desired state of Kany8sCluster
type Kany8sClusterSpec struct {
	// kroSpec is an arbitrary, provider-specific object.
	// +optional
	KroSpec *apiextensionsv1.JSON `json:"kroSpec,omitempty"`
}

// Kany8sClusterInitializationStatus defines fields related to infrastructure
// initialization.
type Kany8sClusterInitializationStatus struct {
	// provisioned denotes whether the infrastructure has been provisioned.
	//
	// This field is required by the Cluster API v1beta2 InfrastructureCluster contract.
	// +optional
	Provisioned bool `json:"provisioned,omitempty"`
}

// Kany8sClusterStatus defines the observed state of Kany8sCluster.
type Kany8sClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// initialization holds fields related to infrastructure initialization.
	// +optional
	Initialization Kany8sClusterInitializationStatus `json:"initialization,omitzero"`

	// conditions represent the current state of the Kany8sCluster resource.
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

	// failureReason will be set in the event that there is a terminal problem
	// reconciling infrastructure and will contain a succinct reason for the
	// failure.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// failureMessage will be set in the event that there is a terminal problem
	// reconciling infrastructure and will contain a more verbose string suitable
	// for logging and human consumption.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1alpha1"

// Kany8sCluster is the Schema for the kany8sclusters API
type Kany8sCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Kany8sCluster
	// +required
	Spec Kany8sClusterSpec `json:"spec"`

	// status defines the observed state of Kany8sCluster
	// +optional
	Status Kany8sClusterStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// Kany8sClusterList contains a list of Kany8sCluster
type Kany8sClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Kany8sCluster `json:"items"`
}

func (r *Kany8sCluster) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Kany8sCluster) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Kany8sCluster{}, &Kany8sClusterList{})
}
