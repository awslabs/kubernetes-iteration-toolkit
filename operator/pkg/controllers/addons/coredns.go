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

package addons

import (
	"context"

	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/ptr"
)

const (
	clusterIP    = "10.96.0.10"
	coreDNSImage = "public.ecr.aws/eks-distro/coredns/coredns:v1.8.3-eks-1-20-4"
)

type reconcileResources func(context.Context) (err error)

func (c *Controller) reconcileCoreDNS(ctx context.Context, _ *Controller, controlPlane *v1alpha1.ControlPlane) error {
	for _, reconcile := range []reconcileResources{
		c.reconcileServiceAccount,
		c.reconcileClusterRole,
		c.reconcileClusterRoleBinding,
		c.reconcileService,
		c.reconcileConfigMap,
		c.reconcileDeployment,
	} {
		if err := reconcile(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) reconcileServiceAccount(ctx context.Context) error {
	return c.kubeClient.EnsurePatch(ctx, &v1.ServiceAccount{}, &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: kubeSystem,
		},
	})
}

func (c *Controller) reconcileClusterRole(ctx context.Context) error {
	return c.kubeClient.EnsureCreate(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:coredns",
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"endpoints", "services", "pods", "namespaces"},
			Verbs:     []string{"list", "watch"},
		}, {
			APIGroups: []string{""},
			Resources: []string{"nodes"},
			Verbs:     []string{"get"},
		}},
	})
}

func (c *Controller) reconcileClusterRoleBinding(ctx context.Context) error {
	return c.kubeClient.EnsureCreate(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:coredns",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:coredns",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      "coredns",
			Namespace: kubeSystem,
		}},
	})
}

func (c *Controller) reconcileService(ctx context.Context) error {
	return c.kubeClient.EnsureCreate(ctx, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-dns",
			Namespace: kubeSystem,
			Labels: map[string]string{
				"k8s-app":                       "kube-dns",
				"kubernetes.io/cluster-service": "true",
				"kubernetes.io/name":            "CoreDNS",
			},
			Annotations: map[string]string{
				"prometheus.io/port":   "9153",
				"prometheus.io/scrape": "true",
			},
		},
		Spec: v1.ServiceSpec{
			ClusterIP: clusterIP,
			Selector:  coreDNSLabels(),
			Ports: []v1.ServicePort{{
				Name:       "dns",
				Protocol:   "UDP",
				Port:       53,
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 53},
			}, {
				Name:       "dns-tcp",
				Protocol:   "TCP",
				Port:       53,
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 53},
			}, {
				Name:       "metrics",
				Protocol:   "TCP",
				Port:       9153,
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 9153},
			}},
		},
	})
}

const coreDNSConfigData = `.:53 {
	errors
	health {
	   lameduck 5s
	}
	ready
	kubernetes cluster.local in-addr.arpa ip6.arpa {
	   pods insecure
	   fallthrough in-addr.arpa ip6.arpa
	   ttl 30
	}
	prometheus :9153
	forward . /etc/resolv.conf
	cache 30
	loop
	reload
	loadbalance
}`

func (c *Controller) reconcileConfigMap(ctx context.Context) error {
	return c.kubeClient.EnsureCreate(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: kubeSystem,
		},
		Data: map[string]string{
			"Corefile": coreDNSConfigData,
		},
	})
}

func (c *Controller) reconcileDeployment(ctx context.Context) error {
	return c.kubeClient.EnsurePatch(ctx, &appsv1.Deployment{}, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: kubeSystem,
			Labels:    coreDNSLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.Int32(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: coreDNSLabels(),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: coreDNSLabels(),
				},
				Spec: v1.PodSpec{
					PriorityClassName:  "system-cluster-critical",
					ServiceAccountName: "coredns",
					Containers: []v1.Container{{
						Name:            "coredns",
						Image:           coreDNSImage,
						ImagePullPolicy: v1.PullIfNotPresent,
						Resources: v1.ResourceRequirements{
							Requests: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU:    resource.MustParse("1"),
								v1.ResourceMemory: resource.MustParse("70"),
							},
							Limits: map[v1.ResourceName]resource.Quantity{
								v1.ResourceCPU: resource.MustParse("1.7"),
							},
						},
						Args: []string{"-conf", "/etc/coredns/Corefile"},
						Ports: []v1.ContainerPort{{
							Name:          "dns",
							ContainerPort: 53,
							Protocol:      "UDP",
						}, {
							Name:          "dns-tcp",
							ContainerPort: 53,
							Protocol:      "TCP",
						}, {
							Name:          "metrics",
							ContainerPort: 9153,
							Protocol:      "TCP",
						}},
						SecurityContext: &v1.SecurityContext{
							AllowPrivilegeEscalation: ptr.Bool(false),
							Capabilities: &v1.Capabilities{
								Add:  []v1.Capability{"NET_BIND_SERVICE"},
								Drop: []v1.Capability{"all"},
							},
							ReadOnlyRootFilesystem: ptr.Bool(true),
						},
						VolumeMounts: []v1.VolumeMount{{
							Name:      "config-volume",
							MountPath: "/etc/coredns",
							ReadOnly:  true,
						}},
					}},
					Volumes: []v1.Volume{{
						Name: "config-volume",
						VolumeSource: v1.VolumeSource{
							ConfigMap: &v1.ConfigMapVolumeSource{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "coredns",
								},
								Items: []v1.KeyToPath{{
									Key:  "Corefile",
									Path: "Corefile",
								}},
							},
						},
					}},
				},
			},
		},
	})
}

func coreDNSLabels() map[string]string {
	return map[string]string{
		"k8s-app": "kube-dns",
	}
}
