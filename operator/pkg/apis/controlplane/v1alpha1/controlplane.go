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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: if you make changes to this file, run `make codegen` to update the
// appropriate crds and yamls.

// ControlPlane is the Schema for the ControlPlanes API
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=cp
// +kubebuilder:subresource:status
type ControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ControlPlaneSpec   `json:"spec,omitempty"`
	Status ControlPlaneStatus `json:"status,omitempty"`
}

// ControlPlaneList contains a list of ControlPlane
// +kubebuilder:object:root=true
type ControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ControlPlane `json:"items"`
}

// ControlPlaneSpec specifies the shape of the cluster and how components like
// master and etcd are configured to run. By default, KIT uses all the default
// values and ControlPlaneSpec can be empty.
type ControlPlaneSpec struct {
	KubernetesVersion         string `json:"kubernetesVersion,omitempty"`
	ColocateAPIServerWithEtcd bool   `json:"colocateAPIServerWithEtcd,omitempty"`
	// TTL is the duration for which control plane resources are active, once expired resource will be automatically deleted by the operator
	TTL    string     `json:"ttl,omitempty"`
	Master MasterSpec `json:"master,omitempty"`
	Etcd   Etcd       `json:"etcd,omitempty"`
}

type Etcd struct {
	Component                 `json:",inline"`
	PersistentVolumeClaimSpec *v1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec,omitempty"`
}

// MasterSpec provides a way for the user to configure master instances and
// custom flags for components running on master nodes like apiserver, KCM and
// scheduler.
type MasterSpec struct {
	// Provide a KMS key ID to enable the encryption provider
	KMSKeyID *string `json:"kmsKeyId,omitempty"`
	// The EncryptionProvider spec is used only if KMSKeyID is provided.
	EncryptionProvider *Component `json:"encryptionProvider,omitempty"`

	Scheduler         *Component `json:"scheduler,omitempty"`
	ControllerManager *Component `json:"controllerManager,omitempty"`
	APIServer         *Component `json:"apiServer,omitempty"`
	Authenticator     *Component `json:"authenticator,omitempty"`
}

// Component provides a generic way to pass in args and images to master and etcd
// components. If a user wants to change the QPS they need to provide the
// following flag with the desired value -`kube-api-qps:100` in the args.
type Component struct {
	Replicas int         `json:"replicas,omitempty"`
	Spec     *v1.PodSpec `json:"spec,omitempty"`
}

func (c *ControlPlane) ClusterName() string {
	return c.Name
}
