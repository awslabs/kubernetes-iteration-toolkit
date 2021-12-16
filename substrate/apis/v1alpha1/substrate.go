package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SubstrateSpec struct {
	// +optional
	InstanceType *string `json:"instanceType,omitempty"`
}

type SubstrateStatus struct {
	VPCID *string `json:"vpcId,omitempty`
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
