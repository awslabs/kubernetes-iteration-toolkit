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
	"context"
	"crypto/x509"
	"fmt"
	"net"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	pkiutil "github.com/awslabs/kit/operator/pkg/utils/pki"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	certutil "k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	etcdRootCACommonName = "etcd/ca"
)

type Provider struct {
	kubeClient client.Client
}

type certRequest struct {
	*pkiutil.CertConfig
	Name   string
	IsCA   bool
	caCert []byte
	caKey  []byte
}

func New(kubeclient client.Client) *Provider {
	return &Provider{kubeClient: kubeclient}
}

func (e *Provider) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// generate etcd service object
	etcdServiceName, err := e.createETCDService(ctx, controlPlane)
	if err != nil {
		return err
	}
	// Create ETCD certs and keys, store them as secret in the management server
	if err := e.createETCDCerts(ctx, controlPlane); err != nil {
		return err
	}
	// TODO generate etcd stateful set
	_ = etcdServiceName
	return nil
}

func (e *Provider) createETCDService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (string, error) {
	objName := etcdServiceNameFor(controlPlane.ClusterName())
	if err := e.createService(ctx, controlPlane); err != nil {
		return "", fmt.Errorf("creating etcd service object for cluster %v, %w", controlPlane.ClusterName(), err)
	}
	return objName, nil
}

func (e *Provider) createService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdServiceNameFor(controlPlane.ClusterName()),
			Namespace: controlPlane.NamespaceName(),
			Labels:    etcdLabelFor(controlPlane.ClusterName()),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: controlPlane.APIVersion,
				Name:       controlPlane.Name,
				Kind:       controlPlane.Kind,
				UID:        controlPlane.UID,
			}},
		},
	}
	result, err := controllerutil.CreateOrPatch(ctx, e.kubeClient, svc, func() error {
		// We can't update the Spec field completely as the existing svc object
		// has some defaults set by API server like `Spec.Type: ClusterIP`. If
		// we update Spec field, we will need to set these defaults as well
		// because CreateOrPatch does a reflect.DeepEqual for the existing spec
		// and with our change, calls Patch if they are not equal.
		svc.Spec.Selector = etcdLabelFor(controlPlane.ClusterName())
		svc.Spec.Ports = []v1.ServicePort{{
			Port:       2380,
			Name:       etcdServerPortNameFor(controlPlane.ClusterName()),
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 2380},
			Protocol:   "TCP",
		}, {
			Port:       2379,
			Name:       etcdClientPortNameFor(controlPlane.ClusterName()),
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 2379},
			Protocol:   "TCP",
		}}
		svc.Spec.ClusterIP = "None"
		return nil
	})
	if err != nil {
		return fmt.Errorf("creating service %s/%s, %w", svc.Namespace, svc.Name, err)
	}
	if result != controllerutil.OperationResultNone {
		zap.S().Infof("[%s] service %s %s", controlPlane.ClusterName(), svc.Name, result)
	}
	return nil
}

func (e *Provider) createETCDCerts(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// create the ETCD ROOT CA certs and key
	caCert, caKey, err := e.createSecret(ctx, controlPlane, etcdRootCACertConfig(controlPlane))
	if err != nil {
		return fmt.Errorf("creating root CA for name %v, %w", controlPlane.ClusterName(), err)
	}
	// create etcd server, peer, healthcheck and etcdAPIClient certs and key
	for _, certConfig := range certListFor(controlPlane, caCert, caKey) {
		if _, _, err := e.createSecret(ctx, controlPlane, certConfig); err != nil {
			return fmt.Errorf("creating certs and key name %s, %w,", certConfig.Name, err)
		}
	}
	return nil
}

// createSecret will store root CA and key as the kubernetes secret
func (e *Provider) createSecret(ctx context.Context, controlPlane *v1alpha1.ControlPlane, req *certRequest) (cert, key []byte, err error) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: controlPlane.NamespaceName(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: controlPlane.APIVersion,
				Name:       controlPlane.Name,
				Kind:       controlPlane.Kind,
				UID:        controlPlane.UID,
			}},
		},
		Data: map[string][]byte{},
	}
	result, err := controllerutil.CreateOrPatch(ctx, e.kubeClient, secret, func() (err error) {
		secret.Type = v1.SecretTypeTLS
		req.ExistingCert = secret.Data["tls.crt"]
		req.ExistingKey = secret.Data["tls.key"]
		// create certificate and key if the existing is nil or invalid
		if req.IsCA {
			cert, key, err = pkiutil.RootCA(req.CertConfig)
		} else {
			cert, key, err = pkiutil.GenerateCertAndKey(req.CertConfig, req.caCert, req.caKey)
		}
		secret.Data["tls.crt"] = cert
		secret.Data["tls.key"] = key
		return
	})
	if err == nil {
		if result != controllerutil.OperationResultNone {
			zap.S().Infof("[%s] secret %s %s", controlPlane.ClusterName(), secret.Name, result)
		}
	}
	return
}

func etcdServerPortNameFor(clusterName string) string {
	return fmt.Sprintf("etcd-server-ssl-%s", clusterName)
}

func etcdClientPortNameFor(clusterName string) string {
	return fmt.Sprintf("etcd-client-ssl-%s", clusterName)
}

func etcdServiceNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd", clusterName)
}

func etcdCASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-ca", clusterName)
}

func etcdServerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-server", clusterName)
}

func etcdPeerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-peer", clusterName)
}

func etcdLabelFor(clusterName string) map[string]string {
	return map[string]string{
		"app": etcdServiceNameFor(clusterName),
	}
}

func certListFor(controlPlane *v1alpha1.ControlPlane, caCert, caKey []byte) []*certRequest {
	return []*certRequest{
		etcdServerCertConfig(controlPlane, caCert, caKey),
		etcdPeerCertConfig(controlPlane, caCert, caKey),
	}
}

func etcdRootCACertConfig(controlPlane *v1alpha1.ControlPlane) *certRequest {
	return &certRequest{
		IsCA: true,
		Name: etcdCASecretNameFor(controlPlane.ClusterName()),
		CertConfig: &pkiutil.CertConfig{
			Config: &certutil.Config{
				CommonName: etcdRootCACommonName,
			},
		},
	}
}

/*
DNSNames contains the following entries-
"localhost",
<svcname>.<namespace>.svc.cluster.local
<podname>
<podname>.<svcname>.<namespace>.svc.cluster.local
The last two entries are added for every pod in the cluster
*/
func etcdServerCertConfig(controlPlane *v1alpha1.ControlPlane, caCert, caKey []byte) *certRequest {
	return &certRequest{
		Name:   etcdServerSecretNameFor(controlPlane.ClusterName()),
		caCert: caCert,
		caKey:  caKey,
		CertConfig: &pkiutil.CertConfig{
			Config: &certutil.Config{
				Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
				CommonName:   "etcd",
				Organization: []string{"kubernetes"},
				AltNames: certutil.AltNames{
					DNSNames: append(etcdPodAndHostnames(controlPlane),
						etcdSvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace),
						"localhost"),
					IPs: []net.IP{net.IPv4(127, 0, 0, 1)},
				},
			},
		},
	}
}

/*
DNSNames contains the following entries-
"localhost",
<svcname>.<namespace>.svc.cluster.local
<podname>
<podname>.<svcname>.<namespace>.svc.cluster.local
The last two entries are added for every pod in the cluster
*/
func etcdPeerCertConfig(controlPlane *v1alpha1.ControlPlane, caCert, caKey []byte) *certRequest {
	return &certRequest{
		Name:   etcdPeerSecretNameFor(controlPlane.ClusterName()),
		caCert: caCert,
		caKey:  caKey,
		CertConfig: &pkiutil.CertConfig{
			Config: &certutil.Config{
				Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
				CommonName:   "etcd",
				Organization: []string{"kubernetes"},
				AltNames: certutil.AltNames{
					DNSNames: append(etcdPodAndHostnames(controlPlane),
						etcdSvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace),
						"localhost"),
					IPs: []net.IP{net.IPv4(127, 0, 0, 1)},
				},
			},
		},
	}
}

// Service name if <clustername>-etcd.<namespace>.svc.cluster.local
func etcdSvcFQDN(clusterName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", etcdServiceNameFor(clusterName), namespace)
}

// For a given cluster name example, podnames are <clusternme>-etcd-[0-n-1], and
// hostnames are <podname>.<svcname>.kit.svc.cluster.local
func etcdPodAndHostnames(controlPlane *v1alpha1.ControlPlane) []string {
	result := []string{}
	for i := 0; i < controlPlane.Spec.Etcd.Replicas; i++ {
		podname := fmt.Sprintf("%s-etcd-%d", controlPlane.ClusterName(), i)
		result = append(result, podname, fmt.Sprintf("%s.%s", podname, etcdSvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace)))
	}
	return result
}
