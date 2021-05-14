/*
Licensed under the Apache License, Version 2.0 (the "License"); you may not use
this file except in compliance with the License. You may obtain a copy of the
License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed
under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the
specific language governing permissions and limitations under the License.
*/

package v1alpha1

import (
	"knative.dev/pkg/apis"
)

// ControlPlaneStatus defines the observed state of the ControlPlane of a cluster
type ControlPlaneStatus struct {
	// Conditions is the set of conditions required for this ControlPlane to create
	// its objects, and indicates whether or not those conditions are met.
	// +optional
	Conditions apis.Conditions `json:"conditions,omitempty"`
	// Infrastructure is the status of the infrastructure for the ControlPlane.
	// It gets updated by the infrastructure controller as the resources as are
	// being created in AWS
	Infrastructure Infrastructure `json:"infrastructure,omitempty"`
}

type Infrastructure struct {
	VPCID             string   `json:"vpcID,omitempty"`
	PrivateSubnets    []string `json:"privateSubnets,omitempty"`
	PublicSubnets     []string `json:"publicSubnets,omitempty"`
	InternetGatewayID string   `json:"internetGateway,omitempty"`
	NatGatewayID      string   `json:"natGateway,omitempty"`
}

func (c *ControlPlane) StatusConditions() apis.ConditionManager {
	return apis.NewLivingConditionSet(
		Active,
	).Manage(c)
}

func (c *ControlPlane) GetConditions() apis.Conditions {
	return c.Status.Conditions
}

func (c *ControlPlane) SetConditions(conditions apis.Conditions) {
	c.Status.Conditions = conditions
}
