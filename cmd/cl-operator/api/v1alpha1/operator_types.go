/*
Copyright 2024.

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
)

// StatusType represents the status options for ClusterLink components.
type StatusType string

const (
	// StatusNone means a resource was not created.
	StatusNone StatusType = "none"
	// StatusUpdate means a resource was created but is not ready to use.
	StatusUpdate StatusType = "update"
	// StatusError means an error occurred when the resource was created.
	StatusError StatusType = "error"
	// StatusReady means a resource was created and is ready to use.
	StatusReady StatusType = "ready"
)

// IngressType represents the ingress type for ClusterLink project.
type IngressType string

const (
	// IngressTypeNone means that no ecternal ingress was created by the deployment operator.
	IngressTypeNone IngressType = "none"
	// IngressTypeNodePort means that ecternal ingress from type NodePort wad created by the deployment operator.
	IngressTypeNodePort IngressType = "NodePort"
	// IngressTypeLB means that ecternal ingress from type LoadBalncer wad created by the deployment operator.
	IngressTypeLB IngressType = "LoadBalancer"
)

// OperatorSpec defines the desired state of Operator
type OperatorSpec struct {
	DataPlane DataPlaneSpec `json:"dataplane,omitempty"`
	Ingress   IngressSpec   `json:"ingress,omitempty"`

	// +kubebuilder:validation:Enum=trace;info;warning;debug;error;fatal
	// +kubebuilder:default=info
	LogLevel string `json:"logLevel,omitempty"`

	// +kubebuilder:default="ghcr.io/clusterlink-net"
	ContainerRegistry string `json:"containerRegistry,omitempty"`
	// +kubebuilder:default="latest"
	ImageTag string `json:"imageTag,omitempty"`
}

// OperatorStatus defines the observed state of Operator
type OperatorStatus struct {
	Controlplane ComponentStatus `json:"controlplane,omitempty"`
	Dataplane    ComponentStatus `json:"dataplane,omitempty"`
}

// ComponentStatus defines the status of component in ClusterLink.
type ComponentStatus struct {
	// +kubebuilder:validation:Enum=none;ready;error;update
	// +kubebuilder:default=none
	Status  StatusType `json:"status,omitempty"`
	Message string     `json:"message,omitempty"`
}

// DataPlaneSpec defines the desired state of the dataplane components in ClusterLink.
type DataPlaneSpec struct {
	// +kubebuilder:validation:Enum=envoy;go
	// +kubebuilder:default=envoy
	Type string `json:"type,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=1
	Replicas int `json:"replicas,omitempty"`
}

// IngressSpec defines the type of the ingress component in ClusterLink.
type IngressSpec struct {
	// +kubebuilder:validation:Enum=none;LoadBalancer;NodePort
	// +kubebuilder:default=none
	Type IngressType `json:"type,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Operator is the Schema for the operators API
type Operator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OperatorSpec   `json:"spec,omitempty"`
	Status OperatorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OperatorList contains a list of Operator
type OperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Operator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Operator{}, &OperatorList{})
}
