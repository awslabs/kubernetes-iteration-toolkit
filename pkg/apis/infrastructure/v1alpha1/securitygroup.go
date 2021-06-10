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

// SecurityGroup is the Schema for the SecurityGroups API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type SecurityGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecurityGroupSpec   `json:"spec,omitempty"`
	Status SecurityGroupStatus `json:"status,omitempty"`
}

// SecurityGroupSpec
type SecurityGroupSpec struct {
	GroupName   string          `json:"groupName,omitempty"`
	ClusterName string          `json:"clusterName,omitempty"`
	Permissions []*IpPermission `json:"permissions,omitempty"`
}

// SecurityGroupList contains a list of SecurityGroup
// +kubebuilder:object:root=true
type SecurityGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityGroup `json:"items"`
}

// SecurityGroupStatus defines the observed state of the SecurityGroup of a cluster
type SecurityGroupStatus struct {
	// Conditions is the set of conditions required for this SecurityGroup to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions      apis.Conditions `json:"conditions,omitempty"`
	SecurityGroupID string          `json:"securityGroup,omitempty"`
}

type IpPermission struct {
	FromPort   *int64  `json:"fromPort,omitempty"`
	ToPort     *int64  `json:"toPort,omitempty"`
	IpProtocol *string `json:"ipProtocol,omitempty"`
	CidrIP     *string `json:"cidrIP,omitempty"`
	GroupName  *string `json:"groupID,omitempty"`
}

func (c *SecurityGroup) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *SecurityGroup) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *SecurityGroup) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}

// <foo>-<master-instances>-<security-group>
func GroupName(controlPlaneName, component string) string {
	return fmt.Sprintf("%s-%s-%s", controlPlaneName, component, SecurityGroupKey)
}
