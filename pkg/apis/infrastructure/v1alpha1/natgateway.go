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
	"knative.dev/pkg/apis"
)

// NatGateway is the Schema for the NatGateway API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type NatGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NatGatewaySpec   `json:"spec,omitempty"`
	Status NatGatewayStatus `json:"status,omitempty"`
}

// NatGatewaySpec
type NatGatewaySpec struct {
	// TODO Add CIDR field
}

// NatGatewayList contains a list of NatGateway
// +kubebuilder:object:root=true
type NatGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NatGateway `json:"items"`
}

// NatGatewayStatus defines the observed state of the NatGateway of a cluster
type NatGatewayStatus struct {
	// Conditions is the set of conditions required for this NatGateway to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
	ID         string          `json:"NatGatewayid,omitempty"`
}

func (c *NatGateway) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *NatGateway) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *NatGateway) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}
