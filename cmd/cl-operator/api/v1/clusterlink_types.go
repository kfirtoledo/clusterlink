/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterlinkSpec defines the desired state of Clusterlink
type ClusterlinkSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	DataPlane DataPlaneSpec `json:"dataplane,omitempty"`
	// +kubebuilder:validation:Enum=trace;info;warning;debug;error;fatal
	// +kubebuilder:default=info
	LogLevel string `json:"logLevel,omitempty"`

	// +kubebuilder:default="ghcr.io/clusterlink-net"
	ContainerRegistry string `json:"containerRegistry,omitempty"`
	// +kubebuilder:default="latest"
	ImageTag string `json:"imageTag,omitempty"`
}

// ClusterlinkStatus defines the observed state of Clusterlink
type ClusterlinkStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// DataPlaneSpec defines the desired state of Dataplane in ClusterLink
type DataPlaneSpec struct {
	// +kubebuilder:validation:Enum=envoy;go
	// +kubebuilder:default=envoy
	Type string `json:"type,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=1
	Replicates int `json:"replicates,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Clusterlink is the Schema for the clusterlinks API
type Clusterlink struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterlinkSpec   `json:"spec,omitempty"`
	Status ClusterlinkStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterlinkList contains a list of Clusterlink
type ClusterlinkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Clusterlink `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Clusterlink{}, &ClusterlinkList{})
}
