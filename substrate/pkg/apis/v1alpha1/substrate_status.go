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
	"knative.dev/pkg/apis"
)

type ClusterStatus struct {
	Address               *string `json:"address,omitempty"`
	KubeConfig            *string `json:"kubeConfig,omitempty"`
	LaunchTemplateVersion *string `json:"launchTemplateVersion,omitempty"`
}

type InfrastructureStatus struct {
	VPCID               *string  `json:"vpcID,omitempty"`
	PrivateRouteTableID *string  `json:"privateRouteTableID,omitempty"`
	PublicRouteTableID  *string  `json:"publicRouteTableID,omitempty"`
	SecurityGroupID     *string  `json:"securityGroupID,omitempty"`
	PrivateSubnetIDs    []string `json:"privateSubnetIDs,omitempty"`
	PublicSubnetIDs     []string `json:"publicSubnetIDs,omitempty"`
}

type SubstrateStatus struct {
	Cluster        ClusterStatus        `json:"cluster,omitempty"`
	Infrastructure InfrastructureStatus `json:"infrastructure,omitempty"`
	Conditions     apis.Conditions      `json:"conditions,omitempty"`
}
