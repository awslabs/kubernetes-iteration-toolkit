/*
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

// DataPlane is the Schema for the DataPlanes API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type DataPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataPlaneSpec   `json:"spec,omitempty"`
	Status DataPlaneStatus `json:"status,omitempty"`
}

// DataPlaneList contains a list of DataPlane
// +kubebuilder:object:root=true
type DataPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataPlane `json:"items"`
}

type DataPlaneSpec struct {
	// ClusterName is used to connect the worker nodes to a control plane clusterName.
	ClusterName string `json:"clusterName,omitempty"`
	// NodeCount is the desired number of worker nodes for this dataplane.
	NodeCount int `json:"nodeCount,omitempty"`
	// SubnetSelector lets user define label key and values for kit to select
	// the subnets for worker nodes. It can contain key:value to select subnets
	// with particular label, or a specific key:"*" to select all subnets with a
	// specific key. If no selector is provided, worker nodes are
	// provisioned in the same subnet as control plane nodes.
	// +optional
	SubnetSelector map[string]string `json:"subnetSelector,omitempty"`
	// InstanceTypes is an optional field thats lets user specify the instance
	// types for worker nodes, defaults to instance types "t2.xlarge", "t3.xlarge" or "t3a.xlarge"
	// +optional
	InstanceTypes []string `json:"instanceTypes,omitempty"`
	// InstanceProfile is an optional field thats lets user specify a custom
	// instance profile for the instances created
	// +optional
	InstanceProfile string `json:"instanceProfile,omitempty"`
	// AllocationStrategy helps user define the strategy to provision worker nodes in EC2,
	// defaults to "lowest-price"
	// +optional
	AllocationStrategy string `json:"allocationStrategy,omitempty"`
}
