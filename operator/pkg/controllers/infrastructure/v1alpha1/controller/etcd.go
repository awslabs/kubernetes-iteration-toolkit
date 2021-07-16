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

package controller

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/errors"
	pkiutil "github.com/awslabs/kit/operator/pkg/utils/pki"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	etcdRootCACommonName = "etcd/ca"
)

type etcdProvider struct {
	kubeClient client.Client
}

type CertConfig struct {
	*certutil.Config
	name   string
	isCA   bool
	caCert *x509.Certificate
	caKey  crypto.Signer
}

func newETCDProvider(kubeclient client.Client) *etcdProvider {
	return &etcdProvider{kubeClient: kubeclient}
}

func (e *etcdProvider) deploy(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
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

func (e *etcdProvider) createETCDCerts(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// get CA key and cert
	caCert, caKey, err := e.createCertAndKeyIfNotFound(ctx, controlPlane, etcdRootCACertConfig(controlPlane))
	if err != nil {
		return fmt.Errorf("creating root CA for cluster %v, %w", controlPlane.ClusterName(), err)
	}
	// 	generate etcd server, peer, healthcheck and etcdAPIClient certs
	for _, certConfig := range certListFor(controlPlane, caCert, caKey) {
		if _, _, err := e.createCertAndKeyIfNotFound(ctx, controlPlane, certConfig); err != nil {
			return fmt.Errorf("creating certs and key name %s, %w,", certConfig.name, err)
		}
	}
	return nil
}

// rootCertificateAuthority checks with management server for a root CA secret, if it exists return the cert and key, else
// it will generate a new root ca and store as a secret in management server
func (e *etcdProvider) createCertAndKeyIfNotFound(ctx context.Context, controlPlane *v1alpha1.ControlPlane, certConfig *CertConfig) (*x509.Certificate, crypto.Signer, error) {
	var cert *x509.Certificate
	var key crypto.Signer
	secret, err := e.getSecret(ctx, controlPlane.NamespaceName(), certConfig.name)
	if err != nil {
		if errors.KubeObjNotFound(err) {
			cert, key, err = e.generateCertAndKey(ctx, controlPlane, certConfig)
			if err != nil {
				return nil, nil, err
			}
			return cert, key, err
		}
		return nil, nil, err
	}
	certs, err := certutil.ParseCertsPEM(secret.Data["tls.crt"])
	if err != nil {
		return nil, nil, err
	}
	parsedKey, err := keyutil.ParsePrivateKeyPEM(secret.Data["tls.key"])
	if err != nil {
		return nil, nil, err
	}
	return certs[0], parsedKey.(crypto.Signer), err
}

func (e *etcdProvider) getSecret(ctx context.Context, namespace, objName string) (*v1.Secret, error) {
	result := &v1.Secret{}
	if err := e.kubeClient.Get(ctx, namespacedName(namespace, objName), result); err != nil {
		return nil, fmt.Errorf("getting secret %v, %w", namespacedName(namespace, objName), err)
	}
	return result, nil
}

func (e *etcdProvider) generateCertAndKey(ctx context.Context, controlPlane *v1alpha1.ControlPlane, certConfig *CertConfig) (*x509.Certificate, crypto.Signer, error) {
	cert, key, err := pkiutil.KeyAndCert(certConfig.Config, certConfig.caCert, certConfig.caKey, certConfig.isCA)
	if err != nil {
		return nil, nil, fmt.Errorf("creating certificate authority, %w,", err)
	}
	certBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	keyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey)),
	})
	if err := e.createSecret(ctx, certBytes, keyBytes, controlPlane, certConfig); err != nil {
		return nil, nil, fmt.Errorf("creating root ca secret, %w", err)
	}
	zap.S().Infof("[%s] successfully created cert and key %s", controlPlane.ClusterName(), certConfig.name)
	return cert, key, nil
}

// createSecret will store root CA and key as the kubernetes secret
func (e *etcdProvider) createSecret(ctx context.Context, cert, key []byte, controlPlane *v1alpha1.ControlPlane, certConfig *CertConfig) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certConfig.name,
			Namespace: controlPlane.NamespaceName(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: controlPlane.APIVersion,
				Name:       controlPlane.Name,
				Kind:       controlPlane.Kind,
				UID:        controlPlane.UID,
			}},
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": cert,
			"tls.key": key,
		},
	}
	if err := e.kubeClient.Create(ctx, secret); err != nil {
		return err
	}
	return nil
}

func (e *etcdProvider) createETCDService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (string, error) {
	objName := etcdServiceNameFor(controlPlane.ClusterName())
	service, err := e.getService(ctx, namespacedName(controlPlane.NamespaceName(), objName))
	if err != nil {
		if errors.KubeObjNotFound(err) {
			if err := e.createService(ctx, controlPlane); err != nil {
				return "", fmt.Errorf("creating etcd service object for cluster %v, %w", controlPlane.ClusterName(), err)
			}
			return objName, nil
		}
		return "", fmt.Errorf("getting service from api server, %w", err)
	}
	return service.Name, nil
}

func (e *etcdProvider) createService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
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
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port: 2380,
				Name: etcdServerPortNameFor(controlPlane.ClusterName()),
			}, {
				Port: 2379,
				Name: etcdClientPortNameFor(controlPlane.ClusterName()),
			}},
			ClusterIP: "None",
			Selector:  etcdLabelFor(controlPlane.ClusterName()),
		},
	}
	if err := e.kubeClient.Create(ctx, svc); err != nil {
		return err
	}
	zap.S().Infof("[%s] successfully created service %s", controlPlane.ClusterName(),
		etcdServiceNameFor(controlPlane.ClusterName()))
	return nil
}

func (e *etcdProvider) getService(ctx context.Context, name types.NamespacedName) (*v1.Service, error) {
	result := &v1.Service{}
	if err := e.kubeClient.Get(ctx, name, result); err != nil {
		return nil, fmt.Errorf("getting service %v, %w", name, err)
	}
	return result, nil
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

func namespacedName(namespace, obj string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: obj}
}

func certListFor(controlPlane *v1alpha1.ControlPlane, caCert *x509.Certificate, caKey crypto.Signer) []*CertConfig {
	return []*CertConfig{
		etcdServerCertConfig(controlPlane, caCert, caKey),
		etcdPeerCertConfig(controlPlane, caCert, caKey),
	}
}

func etcdRootCACertConfig(controlPlane *v1alpha1.ControlPlane) *CertConfig {
	return &CertConfig{
		isCA: true,
		name: etcdCASecretNameFor(controlPlane.ClusterName()),
		Config: &certutil.Config{
			CommonName: etcdRootCACommonName,
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
func etcdServerCertConfig(controlPlane *v1alpha1.ControlPlane, caCert *x509.Certificate, caKey crypto.Signer) *CertConfig {
	config := &CertConfig{
		caCert: caCert,
		caKey:  caKey,
		name:   etcdServerSecretNameFor(controlPlane.ClusterName()),
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
	}
	zap.S().Infof("Server cert DNSNames are %+v", config.Config.AltNames.DNSNames)
	return config
}

/*
DNSNames contains the following entries-
"localhost",
<svcname>.<namespace>.svc.cluster.local
<podname>
<podname>.<svcname>.<namespace>.svc.cluster.local
The last two entries are added for every pod in the cluster
*/
func etcdPeerCertConfig(controlPlane *v1alpha1.ControlPlane, caCert *x509.Certificate, caKey crypto.Signer) *CertConfig {
	config := &CertConfig{
		caCert: caCert,
		caKey:  caKey,
		name:   etcdPeerSecretNameFor(controlPlane.ClusterName()),
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
	}
	zap.S().Infof("Peer cert DNSNames are %+v", config.Config.AltNames.DNSNames)
	return config
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

// func etcdHealthCheckCertConfig() *certutil.Config {
// 	return &certutil.Config{}
// }

// func etcdAPIClientCertConfig() *certutil.Config {
// 	return &certutil.Config{}
// }
