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

// AutoScalingGroup is the Schema for the AutoScalingGroup API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type AutoScalingGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutoScalingGroupSpec   `json:"spec,omitempty"`
	Status AutoScalingGroupStatus `json:"status,omitempty"`
}

// AutoScalingGroupSpec
type AutoScalingGroupSpec struct {
	ClusterName   string `json:"clusterName,omitempty"`
	InstanceCount int    `json:"instanceCount,omitempty"`
}

// AutoScalingGroupList contains a list of AutoScalingGroup
// +kubebuilder:object:root=true
type AutoScalingGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutoScalingGroup `json:"items"`
}

// AutoScalingGroupStatus defines the observed state of the AutoScalingGroup of a cluster
type AutoScalingGroupStatus struct {
	// Conditions is the set of conditions required for this AutoScalingGroup to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
}

func (c *AutoScalingGroup) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *AutoScalingGroup) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *AutoScalingGroup) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}

// <foo-master-instances>-<AutoScalingGroup>
func AutoScalingGroupName(component string) string {
	return fmt.Sprintf("%s-autoscalinggroup", component)
}
