package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SubstrateSpec struct {
	// +optional
	VPC *VPCSpec `json:"vpc,omitempty"`

	// +optional
	InstanceType *string `json:"instanceType,omitempty"`
}

type SubstrateStatus struct {
	VPCID                   *string  `json:"vpcID,omitempty"`
	InternetGatewayID       *string  `json:"internetGatewayID,omitempty"`
	ElasticIPIDForNAT       *string  `json:"elasticIPIDForNat,omitempty"`
	ElasticIPIDForAPIServer *string  `json:"elasticIPIDForAPIServer,omitempty"`
	ElasticIPForAPIServer   *string  `json:"elasticIPIDForAPIServer,omitempty"`
	NatGatewayID            *string  `json:"natGatewayID,omitempty"`
	PrivateRouteTableID     *string  `json:"privateRouteTableID,omitempty"`
	PublicRouteTableID      *string  `json:"publicRouteTableID,omitempty"`
	SecurityGroupID         *string  `json:"securityGroupID,omitempty"`
	PrivateSubnetIDs        []string `json:"privateSubnetIDs,omitempty"`
	PublicSubnetIDs         []string `json:"publicSubnetIDs,omitempty"`
}

// Substrate is the Schema for the Substrates API
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=substrates,scope=Cluster
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
