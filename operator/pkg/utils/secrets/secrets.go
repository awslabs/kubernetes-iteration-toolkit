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

package secrets

import (
	"context"
	"fmt"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	pkiutil "github.com/awslabs/kit/operator/pkg/pki"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	certutil "k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Provider struct {
	kubeClient client.Client
}

type CA struct {
	Provider
	caName       string
	caCommonName string
	caCert       []byte
	caKey        []byte
}

type Request struct {
	*pkiutil.CertConfig
	Name string
	IsCA bool
}

func New(kubeClient client.Client) *Provider {
	return &Provider{kubeClient: kubeClient}
}

// WithRootCA returns a secrets provider with the provided CA commonName. Root CA is
// not created immediately, only when a cert and key needs to be signed root CA
// is generated.
func (p *Provider) WithRootCAName(secretName, commonName string) *CA {
	return &CA{Provider: Provider{p.kubeClient}, caName: secretName, caCommonName: commonName}
}

// CreateSecrets loops through all the requests and creates the secrets objects for these requests
// If a root CA is not present, it will create a root CA
func (c *CA) CreateSecrets(ctx context.Context, controlPlane *v1alpha1.ControlPlane, req ...*Request) error {
	for _, r := range req {
		// If root CA doesn't exists, generate one
		if len(c.caCert) == 0 || len(c.caKey) == 0 {
			if err := c.generateCA(ctx, controlPlane); err != nil {
				return fmt.Errorf("creating root CA %v for %v, %w", c.caName, c.caCommonName, err)
			}
		}
		if _, _, err := c.create(ctx, controlPlane, r); err != nil {
			return fmt.Errorf("creating secret %v for %v, %w", r.Name, r.CommonName, err)
		}
	}
	return nil
}

// generateCA creates a new CA if there is no secret found in the cluster with caSecretName
func (c *CA) generateCA(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	c.caCert, c.caKey, err = c.create(ctx, controlPlane, &Request{
		IsCA: true,
		Name: c.caName,
		CertConfig: &pkiutil.CertConfig{
			Config: &certutil.Config{
				CommonName: c.caCommonName,
			},
		},
	})
	return
}

// create creates a v1.Secret object that contains the cert and key and is
// stored in Kubernetes cluster. If the secret object is found in the cluster,
// it reuses the existing the existing cert and key if it is valid.
func (c *CA) create(ctx context.Context, controlPlane *v1alpha1.ControlPlane, req *Request) (cert, key []byte, err error) {
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
	result, err := controllerutil.CreateOrPatch(ctx, c.kubeClient, secret, func() (err error) {
		secret.Type = v1.SecretTypeTLS
		req.ExistingCert = secret.Data["tls.crt"]
		req.ExistingKey = secret.Data["tls.key"]
		// create certificate and key if the existing is nil or invalid
		if req.IsCA {
			cert, key, err = pkiutil.RootCA(req.CertConfig)
		} else {
			cert, key, err = pkiutil.GenerateCertAndKey(req.CertConfig, c.caCert, c.caKey)
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
