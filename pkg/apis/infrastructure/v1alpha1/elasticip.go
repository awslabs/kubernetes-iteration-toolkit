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

// ElasticIP is the Schema for the ElasticIP API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type ElasticIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElasticIPSpec   `json:"spec,omitempty"`
	Status ElasticIPStatus `json:"status,omitempty"`
}

// ElasticIPSpec
type ElasticIPSpec struct {
	// TODO Add CIDR field
}

// ElasticIPList contains a list of ElasticIP
// +kubebuilder:object:root=true
type ElasticIPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElasticIP `json:"items"`
}

// ElasticIPStatus defines the observed state of the ElasticIP of a cluster
type ElasticIPStatus struct {
	// Conditions is the set of conditions required for this ElasticIP to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
	PublicIP   string          `json:"publicIP,omitempty"`
}

func (c *ElasticIP) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *ElasticIP) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *ElasticIP) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}
