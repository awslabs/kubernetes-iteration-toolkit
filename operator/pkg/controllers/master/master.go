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
	"github.com/awslabs/kit/operator/pkg/utils/secrets"

	pkiutil "github.com/awslabs/kit/operator/pkg/pki"
	certutil "k8s.io/client-go/util/cert"

	"sigs.k8s.io/controller-runtime/pkg/client"
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

	// TODO Create a service for master
	if err := c.createSecrets(ctx, controlPlane); err != nil {
		return err
	}
	// TODO Create and apply objects for kube apiserver, KCM and scheduler
	return nil
}

// createMasterSecrets creates the kubernetes secrets containing all the certs
// and key required to run master API server
func (c *Controller) createSecrets(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// create the root CA, certs and key for API server and kubelet client
	if err := c.secretsProvider.WithRootCAName(masterCASecretNameFor(controlPlane.ClusterName()),
		masterCACommonName).CreateSecrets(ctx, controlPlane, certListFor(controlPlane)...); err != nil {
		return err
	}
	// create the root CA, certs and key for front proxy client
	if err := c.secretsProvider.WithRootCAName(kubeFrontProxyCASecretNameFor(controlPlane.ClusterName()),
		frontProxyCACommonName).CreateSecrets(ctx, controlPlane, kubeFrontProxyClient(controlPlane)); err != nil {
		return err
	}
	return nil
}

func certListFor(controlPlane *v1alpha1.ControlPlane) []*secrets.Request {
	return []*secrets.Request{
		kubeAPIServerCertConfig(controlPlane),
		kubeletClientCertConfig(controlPlane),
	}
}

func kubeAPIServerCertConfig(controlPlane *v1alpha1.ControlPlane) *secrets.Request {
	return &secrets.Request{
		Name: kubeAPIServerSecretNameFor(controlPlane.ClusterName()),
		CertConfig: &pkiutil.CertConfig{
			Config: &certutil.Config{
				Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				CommonName: "kube-apiserver",
				AltNames: certutil.AltNames{
					DNSNames: append([]string{
						// TODO add master loadbalancer DNS name here
						"localhost"},
						"kubernetes",
						"kubernetes.default",
						"kubernetes.default.svc",
						"kubernetes.default.svc.cluster.local"),
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
