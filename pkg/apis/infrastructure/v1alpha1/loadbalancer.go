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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
)

// LoadBalancer is the Schema for the LoadBalancer API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerSpec   `json:"spec,omitempty"`
	Status LoadBalancerStatus `json:"status,omitempty"`
}

// LoadBalancerSpec
type LoadBalancerSpec struct {
	ClusterName string `json:"clusterName,omitempty"`
	Type        string `json:"type,omitempty"`
	Scheme      string `json:"scheme,omitempty"`
	Port        int64  `json:"port,omitempty"`
}

// LoadBalancerList contains a list of LoadBalancer
// +kubebuilder:object:root=true
type LoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancer `json:"items"`
}

// LoadBalancerStatus defines the observed state of the LoadBalancer of a cluster
type LoadBalancerStatus struct {
	// Conditions is the set of conditions required for this LoadBalancer to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
}

func (c *LoadBalancer) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *LoadBalancer) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *LoadBalancer) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}

// <foo-master-instances>-<LoadBalancer>
func LoadBalancerName(component string) string {
	return fmt.Sprintf("%s-LoadBalancer", component)
}
