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

// InternetGateway is the Schema for the InternetGateway API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type InternetGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InternetGatewaySpec   `json:"spec,omitempty"`
	Status InternetGatewayStatus `json:"status,omitempty"`
}

// InternetGatewaySpec
type InternetGatewaySpec struct {
}

// InternetGatewayList contains a list of InternetGateway
// +kubebuilder:object:root=true
type InternetGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InternetGateway `json:"items"`
}

// InternetGatewayStatus defines the observed state of the InternetGateway of a cluster
type InternetGatewayStatus struct {
	// Conditions is the set of conditions required for this InternetGateway to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions        apis.Conditions `json:"conditions,omitempty"`
	InternetGatewayID string          `json:"internetGateway,omitempty"`
}

func (c *InternetGateway) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *InternetGateway) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *InternetGateway) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}
