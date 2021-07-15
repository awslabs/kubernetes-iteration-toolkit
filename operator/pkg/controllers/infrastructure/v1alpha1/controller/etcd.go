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
	caCert, caKey, err := e.rootCertificateAuthority(ctx, controlPlane)
	if err != nil {
		return fmt.Errorf("creating root CA for cluster %v, %w", controlPlane.ClusterName(), err)
	}
	// 	TODO generate etcd server, peer, healthcheck and etcdAPIClient certs
	_ = caCert
	_ = caKey
	return nil
}

// rootCertificateAuthority checks with management server for a root CA secret, if it exists return the cert and key, else
// it will generate a new root ca and store as a secret in management server
func (e *etcdProvider) rootCertificateAuthority(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (*x509.Certificate, crypto.Signer, error) {
	var cert *x509.Certificate
	var key crypto.Signer
	objName := etcdCASecretNameFor(controlPlane.ClusterName())
	secret, err := e.getSecret(ctx, namespacedName(controlPlane.NamespaceName(), objName))
	if err != nil {
		if errors.KubeObjNotFound(err) {
			// create a root CA
			cert, key, err = e.createRootCertificateAuthority(ctx, controlPlane)
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

func (e *etcdProvider) getSecret(ctx context.Context, name types.NamespacedName) (*v1.Secret, error) {
	result := &v1.Secret{}
	if err := e.kubeClient.Get(ctx, name, result); err != nil {
		return nil, fmt.Errorf("getting secret %v, %w", name, err)
	}
	return result, nil
}

func (e *etcdProvider) createRootCertificateAuthority(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (*x509.Certificate, crypto.Signer, error) {
	// create a root ca for ETCD
	certConfig := &certutil.Config{CommonName: etcdRootCACommonName}
	cert, key, err := pkiutil.KeyAndCert(certConfig, nil, nil, true)
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
	if err := e.createSecret(ctx, certBytes, keyBytes, controlPlane); err != nil {
		return nil, nil, fmt.Errorf("creating root ca secret, %w", err)
	}
	zap.S().Infof("[%s] successfully created root ca %s", controlPlane.ClusterName(), etcdRootCACommonName)
	return cert, key, nil
}

// createSecret will store root CA and key as the kubernetes secret
func (e *etcdProvider) createSecret(ctx context.Context, cert, key []byte, controlPlane *v1alpha1.ControlPlane) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdCASecretNameFor(controlPlane.ClusterName()),
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
			Selector: etcdLabelFor(controlPlane.ClusterName()),
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

func etcdLabelFor(clusterName string) map[string]string {
	return map[string]string{
		"app": etcdServiceNameFor(clusterName),
	}
}

func namespacedName(namespace, obj string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: obj}
}
