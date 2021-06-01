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

// Role is the Schema for the Role API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Role struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoleSpec   `json:"spec,omitempty"`
	Status RoleStatus `json:"status,omitempty"`
}

// RoleSpec
type RoleSpec struct {
	ClusterName string `json:"clusterName,omitempty"`
	RoleName    string `json:"roleName,omitempty"`
}

// RoleList contains a list of Role
// +kubebuilder:object:root=true
type RoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Role `json:"items"`
}

// RoleStatus defines the observed state of the Role of a cluster
type RoleStatus struct {
	// Conditions is the set of conditions required for this Role to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
}

func (c *Role) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *Role) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *Role) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}

// <foo>-<master-instances>-<role>
func RoleName(controlPlaneName, component string) string {
	return fmt.Sprintf("%s-%s-role", controlPlaneName, component)
}
