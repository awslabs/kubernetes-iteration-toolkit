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
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/imageprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/secrets"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func podSpecFor(controlPlane *v1alpha1.ControlPlane) *v1.PodSpec {
	return &v1.PodSpec{
		TerminationGracePeriodSeconds: aws.Int64(1),
		HostNetwork:                   true,
		DNSPolicy:                     v1.DNSClusterFirstWithHostNet,
		NodeSelector:                  labels(controlPlane.ClusterName()),
		TopologySpreadConstraints: []v1.TopologySpreadConstraint{{
			MaxSkew:           int32(1),
			TopologyKey:       "topology.kubernetes.io/zone",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: labels(controlPlane.ClusterName()),
			},
		}, {
			MaxSkew:           int32(1),
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: labels(controlPlane.ClusterName()),
			},
		}},
		Containers: []v1.Container{{
			Name:  "etcd",
			Image: imageprovider.ETCD(),
			Ports: []v1.ContainerPort{{
				ContainerPort: 2379,
				Name:          "etcd",
			}, {
				ContainerPort: 2380,
				Name:          "etcd-peer",
			}, {
				ContainerPort: 2381,
				Name:          "metrics",
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
			}, {
				Name:      "etcd-bootstrap",
				MountPath: "/etc/kubernetes/",
			}},
			Command: []string{
				"./etc/kubernetes/bootstrap.sh",
			},
			Args: []string{
				"--cert-file=/etc/kubernetes/pki/etcd/server/server.crt",
				"--initial-cluster=" + initialClusterFlag(controlPlane),
				"--data-dir=/var/lib/etcd",
				"--initial-cluster-token=etcd-cluster-1",
				"--key-file=/etc/kubernetes/pki/etcd/server/server.key",
				"--advertise-client-urls=" + advertizeClusterURL(controlPlane),
				"--initial-advertise-peer-urls=" + advertizePeerURL(controlPlane),
				"--listen-client-urls=https://$(NODE_IP):2379,https://127.0.0.1:2379",
				"--listen-metrics-urls=http://$(NODE_IP):2381,http://127.0.0.1:2381",
				"--listen-peer-urls=https://$(NODE_IP):2380",
				"--name=$(NODE_ID)",
				"--peer-cert-file=/etc/kubernetes/pki/etcd/peer/peer.crt",
				"--peer-client-cert-auth=true",
				"--peer-key-file=/etc/kubernetes/pki/etcd/peer/peer.key",
				"--peer-trusted-ca-file=/etc/kubernetes/pki/ca.crt",
				"--snapshot-count=10000",
				"--trusted-ca-file=/etc/kubernetes/pki/ca.crt",
				"--logger=zap",
				"--quota-backend-bytes=8589934592",
				"--max-request-bytes=1572864",
				"--grpc-keepalive-interval=2h0m0s",
				"--pre-vote=true",
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
			LivenessProbe: &v1.Probe{
				ProbeHandler: v1.ProbeHandler{
					HTTPGet: &v1.HTTPGetAction{
						Host:   "127.0.0.1",
						Scheme: v1.URISchemeHTTP,
						Path:   "/health",
						Port:   intstr.FromInt(2381),
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       5,
				TimeoutSeconds:      5,
				FailureThreshold:    5,
			},
			Resources: v1.ResourceRequirements{Requests: v1.ResourceList{
				v1.ResourceCPU: resource.MustParse("1"), v1.ResourceMemory: resource.MustParse("4Gi"),
			}},
		}},
		Volumes: []v1.Volume{{
			Name: "etcd-ca",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  CASecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  secrets.SecretPublicKey,
						Path: "ca.crt",
					}, {
						Key:  secrets.SecretPrivateKey,
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
						Key:  secrets.SecretPublicKey,
						Path: "peer.crt",
					}, {
						Key:  secrets.SecretPrivateKey,
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
						Key:  secrets.SecretPublicKey,
						Path: "server.crt",
					}, {
						Key:  secrets.SecretPrivateKey,
						Path: "server.key",
					}},
				},
			},
		}, {
			Name: "etcd-bootstrap",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					DefaultMode:          aws.Int32(0777),
					LocalObjectReference: v1.LocalObjectReference{Name: bootstrapConfigMapName(controlPlane.ClusterName())},
				},
			},
		}},
	}
}

func initialClusterFlag(controlPlane *v1alpha1.ControlPlane) string {
	nodes := make([]string, 0)
	for i := 0; i < controlPlane.Spec.Etcd.Replicas; i++ {
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

func caServerName(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("%s-etcd-server", controlPlane.ClusterName())
}
func caPeerName(controlPlane *v1alpha1.ControlPlane) string {
	return fmt.Sprintf("%s-etcd-peer", controlPlane.ClusterName())
}
