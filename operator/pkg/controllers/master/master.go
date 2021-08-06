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
	"crypto/x509"
	"fmt"
	"net"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/errors"
	pkiutil "github.com/awslabs/kit/operator/pkg/pki"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	certutil "k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	masterCACommonName     = "kubernetes"
	frontProxyCACommonName = "front-proxy-ca"
)

type Controller struct {
	kubeClient      client.Client
	secretsProvider *secrets.Provider
}

func New(kubeclient client.Client) *Controller {
	return &Controller{kubeClient: kubeclient, secretsProvider: secrets.New(kubeclient)}
}

func (c *Controller) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	// Create a service for master this will create an endpoint for the cluster
	controlPlaneHostname, err := c.getControlPlaneHostname(ctx, controlPlane)
	if err != nil {
		return err
	}
	if err := c.createSecrets(ctx, controlPlaneHostname, controlPlane); err != nil {
		return err
	}
	// TODO Create and apply objects for kube apiserver, KCM and scheduler
	return nil
}

// createMasterSecrets creates the kubernetes secrets containing all the certs
// and key required to run master API server
func (c *Controller) createSecrets(ctx context.Context, hostname string, controlPlane *v1alpha1.ControlPlane) error {
	// create the root CA, certs and key for API server and kubelet client
	if err := c.secretsProvider.WithRootCAName(masterCASecretNameFor(controlPlane.ClusterName()),
		masterCACommonName).CreateSecrets(ctx, controlPlane, certListFor(hostname, controlPlane)...); err != nil {
		return err
	}
	// create the root CA, certs and key for front proxy client
	if err := c.secretsProvider.WithRootCAName(kubeFrontProxyCASecretNameFor(controlPlane.ClusterName()),
		frontProxyCACommonName).CreateSecrets(ctx, controlPlane, kubeFrontProxyClient(controlPlane)); err != nil {
		return err
	}
	return nil
}

func (c *Controller) getControlPlaneHostname(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (string, error) {
	hostname, err := c.createService(ctx, controlPlane)
	if err != nil {
		return "", err
	}
	if hostname == "" {
		return "", fmt.Errorf("waiting for control plane hostname, %w", errors.WaitingForSubResources)
	}
	return hostname, nil
}

func (c *Controller) createService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (string, error) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceNameFor(controlPlane.ClusterName()),
			Namespace: controlPlane.NamespaceName(),
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-scheme":                  "internet-facing",
				"service.beta.kubernetes.io/aws-load-balancer-type":                    "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": "stickiness.enabled=true,stickiness.type=source_ip",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: controlPlane.APIVersion,
				Name:       controlPlane.Name,
				Kind:       controlPlane.Kind,
				UID:        controlPlane.UID,
			}},
		},
	}
	hostname := ""
	result, err := controllerutil.CreateOrPatch(ctx, c.kubeClient, svc, func() error {
		svc.Spec.Selector = labelsFor(controlPlane.ClusterName())
		svc.Spec.Ports = []v1.ServicePort{{
			Port:       443,
			Name:       apiserverPortName(controlPlane.ClusterName()),
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 443},
			Protocol:   "TCP",
		}}
		svc.Spec.Type = v1.ServiceTypeLoadBalancer
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			hostname = svc.Status.LoadBalancer.Ingress[0].Hostname
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("creating service %s/%s, %w", svc.Namespace, svc.Name, err)
	}
	if result != controllerutil.OperationResultNone {
		zap.S().Infof("[%s] service %s %s", controlPlane.ClusterName(), svc.Name, result)
	}
	return hostname, nil
}

func certListFor(hostname string, controlPlane *v1alpha1.ControlPlane) []*secrets.Request {
	return []*secrets.Request{
		kubeAPIServerCertConfig(hostname, controlPlane),
		kubeletClientCertConfig(controlPlane),
	}
}

func kubeAPIServerCertConfig(hostname string, controlPlane *v1alpha1.ControlPlane) *secrets.Request {
	return &secrets.Request{
		Name: kubeAPIServerSecretNameFor(controlPlane.ClusterName()),
		CertConfig: &pkiutil.CertConfig{
			Config: &certutil.Config{
				Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				CommonName: "kube-apiserver",
				AltNames: certutil.AltNames{
					DNSNames: []string{hostname, "localhost", "kubernetes", "kubernetes.default",
						"kubernetes.default.svc", "kubernetes.default.svc.cluster.local"},
					IPs: []net.IP{net.IPv4(127, 0, 0, 1), apiServerVirtualIP()},
				},
			},
		},
	}
}

// Certificate used by the API server to connect to the kubelet
func kubeletClientCertConfig(controlPlane *v1alpha1.ControlPlane) *secrets.Request {
	return &secrets.Request{
		Name: kubeletClientSecretNameFor(controlPlane.ClusterName()),
		CertConfig: &pkiutil.CertConfig{
			Config: &certutil.Config{
				Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				CommonName:   "kube-apiserver-kubelet-client",
				Organization: []string{"system:masters"},
			},
		},
	}
}

// Cert used by the API server to access the front proxy.
func kubeFrontProxyClient(controlPlane *v1alpha1.ControlPlane) *secrets.Request {
	return &secrets.Request{
		Name: kubeFrontProxyClientSecretNameFor(controlPlane.ClusterName()),
		CertConfig: &pkiutil.CertConfig{
			Config: &certutil.Config{
				Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				CommonName: "front-proxy-client",
			},
		},
	}
}

// TODO get this from controlPlane object
func apiServerVirtualIP() net.IP {
	return net.IPv4(10, 96, 0, 1)
}

func masterCASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-master-ca", clusterName)
}

func kubeAPIServerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-apiserver", clusterName)
}

func kubeletClientSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-apiserver-kubelet-client", clusterName)
}

func kubeFrontProxyClientSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-front-proxy-client", clusterName)
}

func kubeFrontProxyCASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-front-proxy-ca", clusterName)
}

func serviceNameFor(clusterName string) string {
	return fmt.Sprintf("%s-controlplane-endpoint", clusterName)
}

func labelsFor(clusterName string) map[string]string {
	return map[string]string{
		"app": serviceNameFor(clusterName),
	}
}

func apiserverPortName(clusterName string) string {
	return fmt.Sprintf("%s-port", serviceNameFor(clusterName))
}
