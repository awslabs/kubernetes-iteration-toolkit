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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/controllers/etcd"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/imageprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/patch"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
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
					MatchLabels: APIServerLabels(controlPlane.ClusterName()),
				},
				Replicas: aws.Int32(int32(controlPlane.Spec.Master.APIServer.Replicas)),
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: APIServerLabels(controlPlane.ClusterName()),
					},
					Spec: apiServerPodSpec,
				},
			},
		}))
}

func APIServerDeploymentName(clusterName string) string {
	return fmt.Sprintf("%s-apiserver", clusterName)
}

func APIServerLabels(clustername string) map[string]string {
	return map[string]string{
		object.AppNameLabelKey:      "apiserver",
		object.ControlPlaneLabelKey: clustername,
	}
}

func apiServerPodSpecFor(controlPlane *v1alpha1.ControlPlane) v1.PodSpec {
	hostPathDirectoryOrCreate := v1.HostPathDirectoryOrCreate
	hostPathDirectory := v1.HostPathDirectory
	return apiServerPodSpecForVersion(controlPlane.Spec.KubernetesVersion, &v1.PodSpec{
		TerminationGracePeriodSeconds: aws.Int64(1),
		HostNetwork:                   true,
		DNSPolicy:                     v1.DNSClusterFirstWithHostNet,
		PriorityClassName:             "system-cluster-critical",
		NodeSelector:                  nodeSelector(controlPlane.ClusterName(), controlPlane.Spec.ColocateAPIServerWithEtcd),
		Affinity:                      affinity(controlPlane.Spec.ColocateAPIServerWithEtcd),
		TopologySpreadConstraints: []v1.TopologySpreadConstraint{{
			MaxSkew:           int32(1),
			TopologyKey:       "topology.kubernetes.io/zone",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: APIServerLabels(controlPlane.ClusterName()),
			},
		}, {
			MaxSkew:           int32(1),
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: APIServerLabels(controlPlane.ClusterName()),
			},
		}},
		Containers: []v1.Container{
			{
				Name:    "apiserver",
				Image:   imageprovider.APIServer(controlPlane.Spec.KubernetesVersion),
				Command: []string{"kube-apiserver"},
				Resources: v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU: resource.MustParse("2"), v1.ResourceMemory: resource.MustParse("6Gi"),
					},
				},
				Ports: []v1.ContainerPort{{ContainerPort: 443, Name: "https"}},
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
					"--kubelet-client-certificate=/etc/kubernetes/pki/kubelet/apiserver-kubelet-client.crt",
					"--kubelet-client-key=/etc/kubernetes/pki/kubelet/apiserver-kubelet-client.key",
					"--kubelet-preferred-address-types=InternalIP,InternalDNS",
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
					"--authentication-token-webhook-config-file=/var/aws-iam-authenticator/kubeconfig/kubeconfig.yaml",
					"--encryption-provider-config=/etc/kubernetes/aws-encryption-provider/encryption-configuration.yaml",
					"--audit-policy-file=/etc/kubernetes/audit-policy/audit-policy.yaml",
					"--audit-log-path=/var/log/kubernetes/audit/" + fmt.Sprintf("%s-%s-$(POD_NAME).log", controlPlane.Namespace, controlPlane.ClusterName()),
					"--audit-log-maxbackup=1",
					"--profiling=true",
					"--cloud-provider=external",
					"--shutdown-delay-duration=5s",
					"--authentication-token-webhook-cache-ttl=7m",
					"--enable-aggregator-routing=true",
					"--tls-cipher-suites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
					"--service-account-max-token-expiration=24h",
					"--kubelet-certificate-authority=/etc/kubernetes/pki/ca/ca.crt",
					"--feature-gates=TTLAfterFinished=true",
					"--logtostderr=true",
					"--v=2",
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
				}, {
					Name: "POD_NAME",
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
				}, {
					Name:      "authenticator-config",
					MountPath: "/var/aws-iam-authenticator/kubeconfig/",
					ReadOnly:  true,
				}, {
					Name:      "var-run-kmsplugin",
					MountPath: "/var/run/kmsplugin/",
				}, {
					Name:      "aws-provider-encryption-config",
					MountPath: "/etc/kubernetes/aws-encryption-provider",
					ReadOnly:  true,
				}, {
					Name:      "audit-log",
					MountPath: "/var/log/kubernetes/audit/",
					ReadOnly:  false,
				}, {
					Name:      "audit-config",
					MountPath: "/etc/kubernetes/audit-policy",
					ReadOnly:  true,
				}},
				LivenessProbe: &v1.Probe{
					ProbeHandler: v1.ProbeHandler{
						HTTPGet: &v1.HTTPGetAction{
							Host:   "127.0.0.1",
							Scheme: v1.URISchemeHTTPS,
							Path:   "/livez",
							Port:   intstr.FromInt(443),
						},
					},
					InitialDelaySeconds: 10,
					PeriodSeconds:       5,
					TimeoutSeconds:      5,
					FailureThreshold:    5,
				},
				ReadinessProbe: &v1.Probe{
					ProbeHandler: v1.ProbeHandler{
						HTTPGet: &v1.HTTPGetAction{
							Host:   "127.0.0.1",
							Scheme: v1.URISchemeHTTPS,
							Path:   "/readyz",
							Port:   intstr.FromInt(443),
						},
					},
					InitialDelaySeconds: 0,
					PeriodSeconds:       5,
					TimeoutSeconds:      5,
					FailureThreshold:    5,
				},
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
		}, {
			Name: "authenticator-config",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/var/aws-iam-authenticator/kubeconfig/",
					Type: &hostPathDirectory,
				},
			},
		}, {
			Name: "var-run-kmsplugin",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/var/run/kmsplugin/",
					Type: &hostPathDirectoryOrCreate,
				},
			},
		}, {
			Name: "aws-provider-encryption-config",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{Name: EncryptionProviderConfigName(controlPlane.ClusterName())},
				},
			},
		}, {
			Name: "audit-log",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/var/log/kubernetes/audit/",
					Type: &hostPathDirectoryOrCreate,
				},
			},
		}, {
			Name: "audit-config",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{Name: AuditLogConfigName(controlPlane.ClusterName())},
				},
			},
		}},
	})
}

func affinity(colocateAPIServerWithEtcd bool) *v1.Affinity {
	if colocateAPIServerWithEtcd {
		return &v1.Affinity{PodAffinity: &v1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{object.AppNameLabelKey: "etcd"},
				},
				TopologyKey: "kubernetes.io/hostname",
			}},
		}}
	}
	return nil
}

func nodeSelector(clusterName string, colocateWithEtcd bool) map[string]string {
	selector := APIServerLabels(clusterName)
	if colocateWithEtcd {
		selector[object.AppNameLabelKey] = object.ColocatedApiServerWithETCDLabelValue
	}
	return selector
}

var (
	disabledFlagsForAPI125 = map[string]struct{}{"--feature-gates": {}}
	disabledFlagsForApi126 = map[string]struct{}{"--feature-gates": {}, "--logtostderr": {}}
)

func apiServerPodSpecForVersion(version string, defaultSpec *v1.PodSpec) v1.PodSpec {
	switch version {
	case "1.25":
		disableFlags(defaultSpec, disabledFlagsForAPI125)
	case "1.26", "1.27":
		disableFlags(defaultSpec, disabledFlagsForApi126)
	}
	return *defaultSpec
}

//Method to disable flags from default specs for a k8s version
func disableFlags(defaultSpec *v1.PodSpec, disabledFlags map[string]struct{}) {
	args := []string{}
	for _, arg := range defaultSpec.Containers[0].Args {
		if _, skip := disabledFlags[strings.Split(arg, "=")[0]]; skip {
			continue
		}
		args = append(args, arg)
	}
	defaultSpec.Containers[0].Args = args
}
