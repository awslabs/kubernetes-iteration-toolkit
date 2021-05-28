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

// Subnet is the Schema for the Subnets API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSpec   `json:"spec,omitempty"`
	Status SubnetStatus `json:"status,omitempty"`
}

// SubnetSpec
type SubnetSpec struct {
	Items []*SubnetProperty `json:"items,omitempty"`
}

type SubnetProperty struct {
	CIDR   string `json:"cidr,omitempty"`
	AZ     string `json:"az,omitempty"`
	Public bool   `json:"public,omitempty"`
}

// SubnetList contains a list of Subnet
// +kubebuilder:object:root=true
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subnet `json:"items"`
}

// SubnetStatus defines the observed state of the Subnet of a cluster
type SubnetStatus struct {
	// Conditions is the set of conditions required for this Subnet to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions     apis.Conditions `json:"conditions,omitempty"`
	PublicSubnets  []string        `json:"publicSubnets,omitempty"`
	PrivateSubnets []string        `json:"privateSubnets,omitempty"`
}

func (c *Subnet) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *Subnet) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *Subnet) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}
