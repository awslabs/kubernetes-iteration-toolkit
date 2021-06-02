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

// S3 is the Schema for the S3 API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type S3 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3Spec   `json:"spec,omitempty"`
	Status S3Status `json:"status,omitempty"`
}

// S3Spec
type S3Spec struct {
	BucketName string `json:"bucketName,omitempty"`
}

// S3List contains a list of S3
// +kubebuilder:object:root=true
type S3List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3 `json:"items"`
}

// S3Status defines the observed state of the S3 of a cluster
type S3Status struct {
	// Conditions is the set of conditions required for this S3 to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
}

func (c *S3) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *S3) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *S3) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}
