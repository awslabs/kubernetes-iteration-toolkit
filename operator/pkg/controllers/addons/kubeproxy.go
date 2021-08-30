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
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/controllers/master"
	"github.com/awslabs/kit/operator/pkg/utils/kubeconfigs"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"
	"knative.dev/pkg/ptr"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	kubeSystem             = "kube-system"
	defaultStr             = "default"
	kubeProxyImage         = "public.ecr.aws/eks-distro/kubernetes/kube-proxy:v1.20.7-eks-1-20-4"
	KubeProxyDaemonSetName = "kubeproxy-daemonset"
)

func (c *Controller) kubeConfigForKubeProxy(ctx context.Context, managerCluster *Controller, controlPlane *v1alpha1.ControlPlane) error {
	// Get the admin config stored in secret in the management cluster
	caSecret, err := managerCluster.keypairs.GetSecretFromServer(ctx,
		object.NamespacedName(master.RootCASecretNameFor(controlPlane.ClusterName()), controlPlane.Namespace))
	if err != nil {
		return fmt.Errorf("getting ca certificate, %w", err)
	}
	endpoint, err := master.GetClusterEndpoint(ctx, managerCluster.kubeClient,
		object.NamespacedName(controlPlane.ClusterName(), controlPlane.Namespace))
	if err != nil {
		return fmt.Errorf("getting cluster endpoint, %w", err)
	}
	// controlPlane is nil as the owner for secret object is not required
	if err := kubeconfigs.Reconciler(c.kubeClient).ReconcileConfigFor(ctx, nil, kubeConfigRequest(
		endpoint, kubeSystem, authRequestFor(controlPlane.ClusterName(), caSecret))); err != nil {
		return fmt.Errorf("reconciling kubeconfig for kube-proxy, %w", err)
	}
	return nil
}

func (c *Controller) daemonsetForKubeProxy(ctx context.Context, _ *Controller, controlPlane *v1alpha1.ControlPlane) (err error) {
	podSpec := kubeProxyPodSpecFor(controlPlane)
	// TODO merge custom flags from the user
	return c.kubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      KubeProxyDaemonSetName,
				Namespace: kubeSystem,
			},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
				Selector: &metav1.LabelSelector{
					MatchLabels: labelsForKubeProxy(),
				},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labelsForKubeProxy(),
					},
					Spec: podSpec,
				},
			},
		},
	)
}

func kubeConfigRequest(endpoint, ns string, auth *authRequest) *kubeconfigs.Request {
	return &kubeconfigs.Request{
		ClusterContext:    defaultStr,
		ClusterName:       defaultStr,
		Namespace:         ns,
		ApiServerEndpoint: endpoint,
		Name:              auth.name,
		AuthInfo:          auth,
		Contexts: map[string]*clientcmdapi.Context{
			defaultStr: {
				Cluster:   defaultStr,
				Namespace: defaultStr,
				AuthInfo:  defaultStr,
			},
		},
	}
}

func authRequestFor(clusterName string, caSecret *v1.Secret) *authRequest {
	_, caCert := secrets.Parse(caSecret)
	return &authRequest{
		name:   KubeProxyConfigNameFor(clusterName),
		caCert: caCert,
	}
}

func KubeProxyConfigNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kubeproxy-config", clusterName)
}

func labelsForKubeProxy() map[string]string {
	return map[string]string{"k8s-app": "kube-proxy"}
}

type authRequest struct {
	name   string
	caCert []byte
}

func (r *authRequest) Generate() (map[string]*clientcmdapi.AuthInfo, error) {
	return map[string]*clientcmdapi.AuthInfo{
		defaultStr: {TokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token"},
	}, nil
}

func (r *authRequest) CACert() []byte {
	return r.caCert
}

func kubeProxyPodSpecFor(controlPlane *v1alpha1.ControlPlane) v1.PodSpec {
	hostPathFileOrCreate := v1.HostPathFileOrCreate
	return v1.PodSpec{
		TerminationGracePeriodSeconds: aws.Int64(1),
		HostNetwork:                   true,
		DNSPolicy:                     v1.DNSClusterFirstWithHostNet,
		PriorityClassName:             "system-node-critical",
		Containers: []v1.Container{
			{
				Name:  "kubeproxy",
				Image: kubeProxyImage,
				Resources: v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU: resource.MustParse("1"),
					},
				},
				SecurityContext: &v1.SecurityContext{
					Privileged: ptr.Bool(true),
				},
				Command: []string{"kube-proxy"},
				Args: []string{
					"--kubeconfig=/var/lib/kube-proxy/kubeconfig",
					"--iptables-min-sync-period=0s",
					"--oom-score-adj=-998",
				},
				VolumeMounts: []v1.VolumeMount{{
					Name:      "varlog",
					MountPath: "/var/log",
				}, {
					Name:      "xtables-lock",
					MountPath: "/run/xtables.lock",
				}, {
					Name:      "lib-modules",
					MountPath: "/lib/modules",
					ReadOnly:  true,
				}, {
					Name:      "kubeproxy-kubeconfig",
					MountPath: "/var/lib/kube-proxy",
					ReadOnly:  true,
				}},
			}},
		Volumes: []v1.Volume{{
			Name: "varlog",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/var/log",
					Type: &hostPathFileOrCreate,
				},
			},
		}, {
			Name: "xtables-lock",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/run/xtables.lock",
					Type: &hostPathFileOrCreate,
				},
			},
		}, {
			Name: "lib-modules",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/lib/modules",
				},
			},
		}, {
			Name: "kubeproxy-kubeconfig",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  KubeProxyConfigNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "config",
						Path: "kubeconfig",
					}},
				},
			},
		}},
	}
}
