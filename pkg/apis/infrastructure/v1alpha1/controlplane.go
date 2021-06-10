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
)

const (
	MasterInstances = "master-instances"
	ETCDInstances   = "etcd-instances"
	WorkerInstances = "worker-instances"

	SecurityGroupKey = "security-group"
)

var ComponentsSupported = []string{ETCDInstances, MasterInstances}

// var ComponentsSupportedWithWorkerNodes = []string{ETCDInstances, MasterInstances, WorkerInstances}

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
	Infrastructure    `json:",inline"`
	KubernetesVersion string     `json:"kubernetesVersion,omitempty"`
	Master            MasterSpec `json:"master,omitempty"`
	Etcd              ETCDSpec   `json:"etcd,omitempty"`
}

type Infrastructure struct {
	VPCCidr        string   `json:"vpccidr,omitempty"`
	Zones          []string `json:"zones,omitempty"`
	PrivateSubnets []string `json:"privateSubnets,omitempty"`
}

// MasterSpec provides a way for the user to configure master instances and
// custom flags for components running on master nodes like apiserver, KCM and
// scheduler.
type MasterSpec struct {
	Instances         `json:",inline"`
	Scheduler         Config `json:"scheduler,omitempty"`
	ControllerManager Config `json:"controllerManager,omitempty"`
	APIServer         Config `json:"apiServer,omitempty"`
}

// ETCDSpec provides a way to configure the etcd nodes and args which are passed to the etcd process.
type ETCDSpec struct {
	Instances `json:",inline"`
	Config    `json:",inline"`
}

// Config provides a generic way to pass in args and images to master and etcd
// components. If a user wants to change the QPS they need to provide the
// following flag with the desired value -`kube-api-qps:100` in the args.
type Config struct {
	Args  map[string]string `json:"args,omitempty"`
	Image string            `json:"image,omitempty"`
}

// Instances denotes how the infrastructure of a particular components looks
// like, if a user wants to use a specific AMI ID, they can provide this in the
// Instances for the corresponding component.
type Instances struct {
	AMI   string `json:"ami,omitempty"`
	Type  string `json:"type,omitempty"`
	Count int    `json:"count,omitempty"`
}

func (c *ControlPlane) MasterSecurePort() string {
	return c.Spec.Master.APIServer.Args["secure-port"]
}

func (c *ControlPlane) MasterSecurePortInt64() int64 {
	_ = c.Spec.Master.APIServer.Args["secure-port"]
	// TODO hardcoded for nowsudo -
	return 443
}
