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

package master

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/controllers/etcd"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/patch"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	apiserverImage        = "public.ecr.aws/eks-distro/kubernetes/kube-apiserver:v1.20.7-eks-1-20-4"
	serviceClusterIPRange = "10.96.0.0/12"
)

func (c *Controller) reconcileApiServer(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	apiServerPodSpec := apiServerPodSpecFor(controlPlane)
	if controlPlane.Spec.Master.APIServer != nil {
		apiServerPodSpec, err = patch.PodSpec(&apiServerPodSpec, controlPlane.Spec.Master.APIServer.Spec)
		if err != nil {
			return fmt.Errorf("patch api server pod spec, %w", err)
		}
	}
	return c.kubeClient.EnsurePatch(ctx, &appsv1.Deployment{},
		object.WithOwner(controlPlane, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      APIServerDeploymentName(controlPlane.ClusterName()),
				Namespace: controlPlane.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: apiServerLabels(controlPlane.ClusterName()),
				},
				Replicas: aws.Int32(3),
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: apiServerLabels(controlPlane.ClusterName()),
					},
					Spec: apiServerPodSpec,
				},
			},
		}))
}

func APIServerDeploymentName(clusterName string) string {
	return fmt.Sprintf("%s-apiserver", clusterName)
}

func apiServerLabels(clustername string) map[string]string {
	return map[string]string{
		object.AppNameLabelKey: APIServerDeploymentName(clustername),
	}
}

func apiServerPodSpecFor(controlPlane *v1alpha1.ControlPlane) v1.PodSpec {
	hostPathDirectoryOrCreate := v1.HostPathDirectoryOrCreate
	return v1.PodSpec{
		TerminationGracePeriodSeconds: aws.Int64(1),
		HostNetwork:                   true,
		DNSPolicy:                     v1.DNSClusterFirstWithHostNet,
		PriorityClassName:             "system-cluster-critical",
		NodeSelector:                  nodeSelector(controlPlane.ClusterName()),
		TopologySpreadConstraints: []v1.TopologySpreadConstraint{{
			MaxSkew:           int32(1),
			TopologyKey:       "topology.kubernetes.io/zone",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: apiServerLabels(controlPlane.ClusterName()),
			},
		}, {
			MaxSkew:           int32(1),
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: apiServerLabels(controlPlane.ClusterName()),
			},
		}},
		Containers: []v1.Container{
			{
				Name:    "apiserver",
				Image:   apiserverImage,
				Command: []string{"kube-apiserver"},
				Resources: v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU: resource.MustParse("1"),
					},
				},
				Args: []string{
					"--advertise-address=$(NODE_IP)",
					"--allow-privileged=true",
					"--authorization-mode=Node,RBAC",
					"--client-ca-file=/etc/kubernetes/pki/ca/ca.crt",
					"--enable-admission-plugins=NodeRestriction",
					"--enable-bootstrap-token-auth=true",
					"--etcd-cafile=/etc/kubernetes/pki/etcd-ca/ca.crt",
					"--etcd-certfile=/etc/kubernetes/pki/etcd/apiserver-etcd-client.crt",
					"--etcd-keyfile=/etc/kubernetes/pki/etcd/apiserver-etcd-client.key",
					"--etcd-servers=https://" + etcd.SvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace) + ":2379",
					"--insecure-port=0",
					"--kubelet-client-certificate=/etc/kubernetes/pki/kubelet/apiserver-kubelet-client.crt",
					"--kubelet-client-key=/etc/kubernetes/pki/kubelet/apiserver-kubelet-client.key",
					"--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
					"--proxy-client-cert-file=/etc/kubernetes/pki/proxy/front-proxy-client.crt",
					"--proxy-client-key-file=/etc/kubernetes/pki/proxy/front-proxy-client.key",
					"--requestheader-allowed-names=front-proxy-client",
					"--requestheader-client-ca-file=/etc/kubernetes/pki/proxy-ca/front-proxy-ca.crt",
					"--requestheader-extra-headers-prefix=X-Remote-Extra-",
					"--requestheader-group-headers=X-Remote-Group",
					"--requestheader-username-headers=X-Remote-User",
					"--secure-port=443",
					"--service-account-issuer=https://kubernetes.default.svc.cluster.local",
					"--service-account-key-file=/etc/kubernetes/pki/sa/sa.pub",
					"--service-account-signing-key-file=/etc/kubernetes/pki/sa/sa.key",
					"--service-cluster-ip-range=" + serviceClusterIPRange,
					"--tls-cert-file=/etc/kubernetes/pki/apiserver/apiserver.crt",
					"--tls-private-key-file=/etc/kubernetes/pki/apiserver/apiserver.key",
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
				VolumeMounts: []v1.VolumeMount{{
					Name:      "ca-certs",
					MountPath: "/etc/ssl/certs",
					ReadOnly:  true,
				}, {
					Name:      "etcd-ca",
					MountPath: "/etc/kubernetes/pki/etcd-ca",
					ReadOnly:  true,
				}, {
					Name:      "client-ca-file",
					MountPath: "/etc/kubernetes/pki/ca",
					ReadOnly:  true,
				}, {
					Name:      "apiserver-etcd-client",
					MountPath: "/etc/kubernetes/pki/etcd",
					ReadOnly:  true,
				}, {
					Name:      "apiserver-kubelet-client",
					MountPath: "/etc/kubernetes/pki/kubelet",
					ReadOnly:  true,
				}, {
					Name:      "front-proxy-client",
					MountPath: "/etc/kubernetes/pki/proxy",
					ReadOnly:  true,
				}, {
					Name:      "front-proxy-ca",
					MountPath: "/etc/kubernetes/pki/proxy-ca",
					ReadOnly:  true,
				}, {
					Name:      "service-account",
					MountPath: "/etc/kubernetes/pki/sa",
					ReadOnly:  true,
				}, {
					Name:      "apiserver",
					MountPath: "/etc/kubernetes/pki/apiserver",
					ReadOnly:  true,
				}},
			}},
		Volumes: []v1.Volume{{
			Name: "ca-certs",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/etc/ssl/certs",
					Type: &hostPathDirectoryOrCreate,
				},
			},
		}, {
			Name: "etcd-ca",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  etcd.CASecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "ca.crt",
					}},
				},
			},
		}, {
			Name: "client-ca-file",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  RootCASecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "ca.crt",
					}, {
						Key:  "private",
						Path: "ca.key",
					}},
				},
			},
		}, {
			Name: "apiserver-etcd-client",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  etcd.EtcdAPIClientSecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "apiserver-etcd-client.crt",
					}, {
						Key:  "private",
						Path: "apiserver-etcd-client.key",
					}},
				},
			},
		}, {
			Name: "apiserver-kubelet-client",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  KubeletClientSecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "apiserver-kubelet-client.crt",
					}, {
						Key:  "private",
						Path: "apiserver-kubelet-client.key",
					}},
				},
			},
		}, {
			Name: "front-proxy-client",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  KubeFrontProxyClientSecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "front-proxy-client.crt",
					}, {
						Key:  "private",
						Path: "front-proxy-client.key",
					}},
				},
			},
		}, {
			Name: "front-proxy-ca",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  FrontProxyCASecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "front-proxy-ca.crt",
					}},
				},
			},
		}, {
			Name: "service-account",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  SAKeyPairSecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "sa.pub",
					}, {
						Key:  "private",
						Path: "sa.key",
					}},
				},
			},
		}, {
			Name: "apiserver",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  KubeAPIServerSecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "apiserver.crt",
					}, {
						Key:  "private",
						Path: "apiserver.key",
					}},
				},
			},
		}},
	}
}
