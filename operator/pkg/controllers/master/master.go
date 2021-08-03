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

type Provider struct {
	kubeClient client.Client
}

func New(kubeclient client.Client) *Provider {
	return &Provider{kubeClient: kubeclient}
}

func (p *Provider) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {

	// TODO Create a service for master NLB
	if err := p.createMasterSecrets(ctx, controlPlane); err != nil {
		return err
	}

	// Create and apply objects for kube apiserver, KCM and scheduler
	return nil
}

// createMasterSecrets creates the kubernetes secrets containing all the certs
// and key required to run master API server
func (p *Provider) createMasterSecrets(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// create the root CA, certs and key for API server and kubelet client
	if err := secrets.WithRootCAName(p.kubeClient,
		masterCASecretNameFor(controlPlane.ClusterName()), masterCACommonName).
		CreateSecrets(ctx, controlPlane, certListFor(controlPlane)...); err != nil {
		return err
	}
	// create the root CA, certs and key for front proxy client
	if err := secrets.WithRootCAName(p.kubeClient,
		kubeFrontProxyCASecretNameFor(controlPlane.ClusterName()), frontProxyCACommonName).
		CreateSecrets(ctx, controlPlane, kubeFrontProxyClient(controlPlane)); err != nil {
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
