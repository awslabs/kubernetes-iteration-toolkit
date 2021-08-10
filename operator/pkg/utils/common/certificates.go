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

package common

import (
	"context"
	"fmt"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type CertificatesProvider struct {
	kubeClient *kubeprovider.Client
}

// CertTree contains root CA as the key and all the certificates signed by
// this root CA (leafs) are added as the value for this map
type CertTree map[*secrets.Request][]*secrets.Request

func New(kubeClient *kubeprovider.Client) *CertificatesProvider {
	return &CertificatesProvider{kubeClient: kubeClient}
}

// ReconcileFor reconciles all certs/key requested as part of the certsTreeMap.
// All the cert/key pairs are stored as a secret object. It will first read the
// existing secret, if not found will create one.
func (c *CertificatesProvider) ReconcileFor(ctx context.Context, certsTreeMap CertTree, controlPlane *v1alpha1.ControlPlane) error {
	for rootCA, leafCerts := range certsTreeMap {
		// Get the existing CA from API server in the form of a Kube secret object,
		// if not found or invalid generate a new one
		caSecret, err := c.GetOrGenerateSecret(ctx, rootCA, nil)
		if err != nil {
			return fmt.Errorf("creating root CA %v, %w", rootCA.Name, err)
		}
		secretObjs := []*v1.Secret{caSecret}
		for _, leafCert := range leafCerts {
			// Get the existing cert and key from API server, if not found or
			// invalid generate a new one
			secretObj, err := c.GetOrGenerateSecret(ctx, leafCert, caSecret)
			if err != nil {
				return fmt.Errorf("creating secret objects %v, %w", secretObj.Name, err)
			}
			secretObjs = append(secretObjs, secretObj)
		}
		for _, secret := range secretObjs {
			if err = c.kubeClient.Ensure(ctx, object.WithOwner(controlPlane, secret)); err != nil {
				return fmt.Errorf("ensuring secret %v, %w", secret.Name, err)
			}
		}
	}
	zap.S().Debugf("[%v] Secrets reconciled", controlPlane.ClusterName())
	return nil
}

// GetOrGenerateSecret will check with API server for this object.
// Calls GetSecretFromServer to get from API server and validate
// If the object is not found, it will create and return a new secret object.
func (c *CertificatesProvider) GetOrGenerateSecret(ctx context.Context,
	request *secrets.Request, caSecret *v1.Secret) (*v1.Secret, error) {
	// get secret from api server
	secret, err := c.GetSecretFromServer(ctx, object.NamespacedName(request.Name, request.Namespace))
	if err != nil && errors.IsNotFound(err) {
		// if not found generate a new secret object
		return secrets.CreateWithCerts(request, caSecret)
	}
	// validate the secret object contains valid secret data
	if err := secrets.IsValid(secret); err != nil {
		return nil, fmt.Errorf("invalid secret object %v/%v, %w", request.Namespace, request.Name, err)
	}
	return secret, err
}

// GetSecretFromServer will get the secret from API server and validate
func (c *CertificatesProvider) GetSecretFromServer(ctx context.Context, nn types.NamespacedName) (*v1.Secret, error) {
	// get secret from api server
	secretObj := &v1.Secret{}
	if err := c.kubeClient.Get(ctx, nn, secretObj); err != nil {
		return nil, err
	}
	return secretObj, nil
}
