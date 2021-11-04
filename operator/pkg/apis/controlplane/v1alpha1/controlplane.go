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
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ControlPlane is the Schema for the ControlPlanes API
// +kubebuilder:object:root=true
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
	KubernetesVersion string     `json:"kubernetesVersion,omitempty"`
	Master            MasterSpec `json:"master,omitempty"`
	Etcd              ETCDSpec   `json:"etcd,omitempty"`
}

// MasterSpec provides a way for the user to configure master instances and
// custom flags for components running on master nodes like apiserver, KCM and
// scheduler.
type MasterSpec struct {
	Instances         `json:",inline"`
	Scheduler         *Component `json:"scheduler,omitempty"`
	ControllerManager *Component `json:"controllerManager,omitempty"`
	APIServer         *Component `json:"apiServer,omitempty"`
}

// ETCDSpec provides a way to configure the etcd nodes and args which are passed to the etcd process.
type ETCDSpec struct {
	Instances `json:",inline"`
	Spec      *v1.PodSpec `json:"spec,omitempty"`
}

// Component provides a generic way to pass in args and images to master and etcd
// components. If a user wants to change the QPS they need to provide the
// following flag with the desired value -`kube-api-qps:100` in the args.
type Component struct {
	Replicas int         `json:"replicas,omitempty"`
	Spec     *v1.PodSpec `json:"spec,omitempty"`
}

// Instances denotes how the infrastructure of a particular components looks
// like, if a user wants to use a specific AMI ID, they can provide this in the
// Instances for the corresponding component.
type Instances struct {
	AMI  string `json:"ami,omitempty"`
	Type string `json:"type,omitempty"`
}

func (c *ControlPlane) ClusterName() string {
	return c.Name
}

type ReconcileFinalize interface {
	Reconcile(context.Context, *ControlPlane) error
	Finalize(context.Context, *ControlPlane) error
}
