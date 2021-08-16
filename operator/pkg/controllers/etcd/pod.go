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

package etcd

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

const (
	defaultEtcdReplicas = 3
	defaultEtcdImage    = "public.ecr.aws/eks-distro/etcd-io/etcd:v3.4.14-eks-1-18-1"
)

func podSpecFor(controlPlane *v1alpha1.ControlPlane) *v1.PodSpec {
	return &v1.PodSpec{
		TerminationGracePeriodSeconds: aws.Int64(1),
		HostNetwork:                   true,
		DNSPolicy:                     v1.DNSClusterFirstWithHostNet,
		Containers: []v1.Container{{
			Name:  "etcd",
			Image: defaultEtcdImage,
			Ports: []v1.ContainerPort{{
				ContainerPort: 2379,
				Name:          "etcd",
			}, {
				ContainerPort: 2380,
				Name:          "etcd-peer",
			}},
			VolumeMounts: []v1.VolumeMount{{
				Name:      "etcd-data",
				MountPath: "/var/lib/etcd",
			}, {
				Name:      "etcd-ca",
				MountPath: "/etc/kubernetes/pki",
			}, {
				Name:      "etcd-peer-certs",
				MountPath: "/etc/kubernetes/pki/etcd/peer",
			}, {
				Name:      "etcd-server-certs",
				MountPath: "/etc/kubernetes/pki/etcd/server",
			}},
			Command: []string{"etcd"},
			Args: []string{
				"--cert-file=/etc/kubernetes/pki/etcd/server/server.crt",
				"--initial-cluster=" + initialClusterFlag(controlPlane),
				"--data-dir=/var/lib/etcd",
				"--initial-cluster-state=new",
				"--initial-cluster-token=etcd-cluster-1",
				"--key-file=/etc/kubernetes/pki/etcd/server/server.key",
				"--advertise-client-urls=" + advertizeClusterURL(controlPlane),
				"--initial-advertise-peer-urls=" + advertizePeerURL(controlPlane),
				"--listen-client-urls=https://$(NODE_IP):2379,https://127.0.0.1:2379",
				"--listen-metrics-urls=http://127.0.0.1:2381",
				"--listen-peer-urls=https://$(NODE_IP):2380",
				"--name=$(NODE_ID)",
				"--peer-cert-file=/etc/kubernetes/pki/etcd/peer/peer.crt",
				"--peer-client-cert-auth=true",
				"--peer-key-file=/etc/kubernetes/pki/etcd/peer/peer.key",
				"--peer-trusted-ca-file=/etc/kubernetes/pki/ca.crt",
				"--snapshot-count=10000",
				"--trusted-ca-file=/etc/kubernetes/pki/ca.crt",
				"--logger=zap",
			},
			Env: []v1.EnvVar{{
				Name: "NODE_IP",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			}, {
				Name: "NODE_ID",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			}},
		}},
		Volumes: []v1.Volume{{
			Name: "etcd-data",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/var/lib/etcd",
				},
			},
		}, {
			Name: "etcd-ca",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  caSecretName(controlPlane),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "tls.crt",
						Path: "ca.crt",
					}, {
						Key:  "tls.key",
						Path: "ca.key",
					}},
				},
			},
		}, {
			Name: "etcd-peer-certs",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  caPeerName(controlPlane),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "tls.crt",
						Path: "peer.crt",
					}, {
						Key:  "tls.key",
						Path: "peer.key",
					}},
				},
			},
		}, {
			Name: "etcd-server-certs",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  caServerName(controlPlane),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "tls.crt",
						Path: "server.crt",
					}, {
						Key:  "tls.key",
						Path: "server.key",
					}},
				},
			},
		}},
	}
}

func initialClusterFlag(controlPlane *v1alpha1.ControlPlane) string {
	nodes := make([]string, 0)
	for i := 0; i < defaultEtcdReplicas; i++ {
		nodes = append(nodes, fmt.Sprintf("%[1]s-etcd-%[2]d=https://%[1]s-etcd-%[2]d.%[1]s-etcd.%[3]s.svc.cluster.local:2380", controlPlane.ClusterName(), i, controlPlane.Namespace))
	}
	return strings.Join(nodes, ",")
}

func advertizeClusterURL(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("https://%s:2379,https://%s:2379", podFQDN(controlPlane), serviceFQDN(controlPlane))
}

func advertizePeerURL(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("https://%s:2380", podFQDN(controlPlane))
}

func podFQDN(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("$(NODE_ID).%s-etcd.%s.svc.cluster.local", controlPlane.ClusterName(), controlPlane.Namespace)
}

func serviceFQDN(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("%s-etcd.%s.svc.cluster.local", controlPlane.ClusterName(), controlPlane.Namespace)
}

func caSecretName(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("%s-etcd-ca", controlPlane.ClusterName())
}

func caServerName(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("%s-etcd-server", controlPlane.ClusterName())
}
func caPeerName(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("%s-etcd-peer", controlPlane.ClusterName())
}
