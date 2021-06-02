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

// TargetGroup is the Schema for the TargetGroup API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type TargetGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetGroupSpec   `json:"spec,omitempty"`
	Status TargetGroupStatus `json:"status,omitempty"`
}

// TargetGroupSpec
type TargetGroupSpec struct {
	ClusterName string `json:"clusterName,omitempty"`
	Port        int64  `json:"port,omitempty"`
}

// TargetGroupList contains a list of TargetGroup
// +kubebuilder:object:root=true
type TargetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TargetGroup `json:"items"`
}

// TargetGroupStatus defines the observed state of the TargetGroup of a cluster
type TargetGroupStatus struct {
	// Conditions is the set of conditions required for this TargetGroup to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
}

func (c *TargetGroup) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *TargetGroup) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *TargetGroup) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}

// <foo-master-instances>-<TargetGroup>
func TargetGroupName(component string) string {
	return fmt.Sprintf("%s-TargetGroup", component)
}
