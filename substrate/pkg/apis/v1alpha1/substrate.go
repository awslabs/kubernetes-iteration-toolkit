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

// +k8s:deepcopy-gen=package,register
// +groupName=kit.sh
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SubstrateSpec struct {
	// +optional
	VPC     *VPCSpec      `json:"vpc,omitempty"`
	Subnets []*SubnetSpec `json:"subnets,omitempty"`
	// +optional
	InstanceType *string `json:"instanceType,omitempty"`
}

type SubstrateStatus struct {
	VPCID               *string  `json:"vpcID,omitempty"`
	ElasticIPID         *string  `json:"elasticIPID,omitempty"`
	PrivateRouteTableID *string  `json:"privateRouteTableID,omitempty"`
	PublicRouteTableID  *string  `json:"publicRouteTableID,omitempty"`
	SecurityGroupID     *string  `json:"securityGroupID,omitempty"`
	PrivateSubnetIDs    []string `json:"privateSubnetIDs,omitempty"`
	PublicSubnetIDs     []string `json:"publicSubnetIDs,omitempty"`
}

// Substrate is the Schema for the Substrates API
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=substrates
// +kubebuilder:subresource:status
type Substrate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubstrateSpec   `json:"spec,omitempty"`
	Status SubstrateStatus `json:"status,omitempty"`
}

type VPCSpec struct {
	// TODO accept a slice of CIDR for megaXL we need to create multiple CIDRs
	CIDR string `json:"cidr,omitempty"`
}

type SubnetSpec struct {
	Zone   string
	CIDR   string
	Public bool
}
