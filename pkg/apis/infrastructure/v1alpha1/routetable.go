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

// RouteTable is the Schema for the RouteTable API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type RouteTable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouteTableSpec   `json:"spec,omitempty"`
	Status RouteTableStatus `json:"status,omitempty"`
}

// RouteTableSpec
type RouteTableSpec struct {
	ClusterName       string `json:"clusterName,omitempty"`
	ForPrivateSubnets bool   `json:"ForPrivateSubnets,omitempty"`
}

// RouteTableList contains a list of RouteTable
// +kubebuilder:object:root=true
type RouteTableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RouteTable `json:"items"`
}

// RouteTableStatus defines the observed state of the RouteTable of a cluster
type RouteTableStatus struct {
	// Conditions is the set of conditions required for this RouteTable to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
	PublicIP   string          `json:"publicIP,omitempty"`
}

func (c *RouteTable) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *RouteTable) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *RouteTable) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}
