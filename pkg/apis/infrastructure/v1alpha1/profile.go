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

// Profile is the Schema for the Profile API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Profile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProfileSpec   `json:"spec,omitempty"`
	Status ProfileStatus `json:"status,omitempty"`
}

// ProfileSpec
type ProfileSpec struct {
	// ClusterName string `json:"clusterName,omitempty"`
	// ProfileName string `json:"ProfileName,omitempty"`
}

// ProfileList contains a list of Profile
// +kubebuilder:object:root=true
type ProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Profile `json:"items"`
}

// ProfileStatus defines the observed state of the Profile of a cluster
type ProfileStatus struct {
	// Conditions is the set of conditions required for this Profile to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
}

func (c *Profile) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *Profile) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *Profile) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}

// <foo-master-instances>-<Profile>
func ProfileName(component string) string {
	return fmt.Sprintf("%s-profile", component)
}
