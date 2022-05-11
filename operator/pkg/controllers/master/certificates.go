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

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/keypairs"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/secrets"

	"k8s.io/apimachinery/pkg/types"
	certutil "k8s.io/client-go/util/cert"
)

const (
	rootCACommonName       = "kubernetes"
	frontProxyCACommonName = "front-proxy-ca"
	//kubeAdminName          = "kubernetes-admin"
)

// reconcileCertificates creates the kubernetes secrets containing all the certs
// and key required to run master API server
func (c *Controller) reconcileCertificates(ctx context.Context, cp *v1alpha1.ControlPlane) error {
	nn := object.NamespacedName(cp.ClusterName(), cp.Namespace)
	endpoint, err := c.getClusterEndpoint(ctx, nn)
	if err != nil {
		return err
	}
	controlPlaneCA := rootCACertConfig(object.NamespacedName(cp.ClusterName(), cp.Namespace))
	frontProxyCA := frontProxyCACertConfig(object.NamespacedName(cp.ClusterName(), cp.Namespace))
	certsTreeMap := keypairs.CertTree{
		controlPlaneCA: {
			kubeAPIServerCertConfig(endpoint, nn),
			kubeletClientCertConfig(nn),
			prometheusClientCertConfig(nn),
		},
		frontProxyCA: {
			kubeFrontProxyClient(nn),
		},
	}
	return c.keypairs.ReconcileCertsFor(ctx, cp, certsTreeMap)
}

func rootCACertConfig(nn types.NamespacedName) *secrets.Request {
	return &secrets.Request{
		Name:      RootCASecretNameFor(nn.Name),
		Namespace: nn.Namespace,
		Type:      secrets.CA,
		Config: &certutil.Config{
			CommonName: rootCACommonName,
		},
	}
}

func kubeAPIServerCertConfig(hostname string, nn types.NamespacedName) *secrets.Request {
	return &secrets.Request{
		Name:      KubeAPIServerSecretNameFor(nn.Name),
		Namespace: nn.Namespace,
		Type:      secrets.KeyWithSignedCert,
		Config: &certutil.Config{
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			CommonName: "kube-apiserver",
			AltNames: certutil.AltNames{
				DNSNames: []string{hostname, "localhost", "kubernetes", "kubernetes.default",
					"kubernetes.default.svc", "kubernetes.default.svc.cluster.local",
					fmt.Sprintf("%s-cp.%s.svc.cluster.local", nn.Name, nn.Namespace),
				},
				IPs: []net.IP{net.IPv4(127, 0, 0, 1), apiServerVirtualIP()},
			},
		},
	}
}

// Certificate used by the API server to connect to the kubelet
func kubeletClientCertConfig(nn types.NamespacedName) *secrets.Request {
	return &secrets.Request{
		Name:      KubeletClientSecretNameFor(nn.Name),
		Namespace: nn.Namespace,
		Type:      secrets.KeyWithSignedCert,
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   "kube-apiserver-kubelet-client",
			Organization: []string{"system:masters"},
		},
	}
}

func frontProxyCACertConfig(nn types.NamespacedName) *secrets.Request {
	return &secrets.Request{
		Name:      FrontProxyCASecretNameFor(nn.Name),
		Namespace: nn.Namespace,
		Type:      secrets.CA,
		Config: &certutil.Config{
			CommonName: frontProxyCACommonName,
		},
	}
}

// Cert used by the API server to access the front proxy.
func kubeFrontProxyClient(nn types.NamespacedName) *secrets.Request {
	return &secrets.Request{
		Name:      KubeFrontProxyClientSecretNameFor(nn.Name),
		Namespace: nn.Namespace,
		Type:      secrets.KeyWithSignedCert,
		Config: &certutil.Config{
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName: "front-proxy-client",
		},
	}
}

// Certificate used by the Prometheus client to scrap API server
func prometheusClientCertConfig(nn types.NamespacedName) *secrets.Request {
	return &secrets.Request{
		Name:      PrometheusClientCertsFor(nn.Name),
		Namespace: nn.Namespace,
		Type:      secrets.KeyWithSignedCert,
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			Organization: []string{"system:monitoring"},
			CommonName:   "system:monitoring",
		},
	}
}

func PrometheusClientCertsFor(clusterName string) string {
	return fmt.Sprintf("%s-prometheus-certs", clusterName)
}

func KubeAPIServerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-apiserver", clusterName)
}

func KubeFrontProxyClientSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-front-proxy-client", clusterName)
}

func KubeletClientSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-apiserver-kubelet-client", clusterName)
}

func RootCASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-controlplane-ca", clusterName)
}

func FrontProxyCASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-front-proxy-ca", clusterName)
}

// TODO get this from controlPlane object
func apiServerVirtualIP() net.IP {
	return net.IPv4(10, 96, 0, 1)
}
