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

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"
	"go.uber.org/zap"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
)

// reconcileKubeConfigs creates required kube configs and stores them as secret in api server
func (c *Controller) reconcileKubeConfigs(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	for _, request := range []*secrets.Request{
		kubeAdminCertConfig(controlPlane.ClusterName()),
		kubeSchedulerCertConfig(controlPlane.ClusterName()),
		kubeControllerManagerCertConfig(controlPlane.ClusterName()),
	} {
		if err := c.reconcileConfigFor(ctx, controlPlane, request); err != nil {
			return err
		}
	}
	zap.S().Debugf("[%v] Kube configs reconciled", controlPlane.ClusterName())
	return nil
}

func (c *Controller) reconcileConfigFor(ctx context.Context, controlPlane *v1alpha1.ControlPlane, request *secrets.Request) error {
	clusterName := controlPlane.ClusterName()
	namespace := controlPlane.Namespace
	endpoint, err := c.getClusterEndpoint(ctx, object.NamespacedName(clusterName, namespace))
	if err != nil {
		return err
	}
	// kubeconfig certs are signed by master root CA certs, if the root CA is not found return
	caSecret, err := c.certificates.GetSecretFromServer(ctx, object.NamespacedName(rootCASecretNameFor(clusterName), namespace))
	if err != nil {
		return err
	}
	// Check if this secret for kubeconfig exists in the api server
	_, err = c.certificates.GetSecretFromServer(ctx, object.NamespacedName(request.Name, namespace))
	if err != nil && errors.IsNotFound(err) {
		// Generate the cert and key for the user
		cert, key, err := secrets.CreateCertAndKey(request, caSecret)
		if err != nil {
			return fmt.Errorf("creating cert and key for %v, %w", request.CommonName, err)
		}
		// generate kubeconfig for this is user and convert to YAML
		configBytes, err := yaml.Marshal(kubeConfigFor(request.CommonName, clusterName, endpoint, caSecret, cert, key))
		if err != nil {
			return fmt.Errorf("marshaling kube config object %v, %w", request.CommonName, err)
		}
		// Create a secret object with config and ensure the secret object in api server
		if err := c.kubeClient.Ensure(ctx, object.WithOwner(controlPlane,
			secrets.CreateWithConfig(object.NamespacedName(request.Name, namespace), configBytes)),
		); err != nil {
			return fmt.Errorf("ensuring kube config for %v, %w", request.CommonName, err)
		}
		return nil
	}
	// TODO validate the existing config in the secret
	return err
}

func kubeConfigFor(userName, clusterName, endpoint string, caSecret *v1.Secret, cert, key []byte) *clientcmdapi.Config {
	contextName := fmt.Sprintf("%s@%s", userName, clusterName)
	caCert, _ := secrets.Parse(caSecret)
	return &clientcmdapi.Config{
		Kind: "Config",
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   endpoint,
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: userName,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			userName: {
				ClientKeyData:         key,
				ClientCertificateData: cert,
			}},
		CurrentContext: contextName,
	}
}

func kubeAdminCertConfig(clusterName string) *secrets.Request {
	return &secrets.Request{
		Name: kubeAdminSecretNameFor(clusterName),
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   "kubernetes-admin",
			Organization: []string{"system:masters"},
		},
	}
}

func kubeSchedulerCertConfig(clusterName string) *secrets.Request {
	return &secrets.Request{
		Name: kubeSchedulerSecretNameFor(clusterName),
		Config: &certutil.Config{
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName: "system:kube-scheduler",
		},
	}
}

func kubeControllerManagerCertConfig(clusterName string) *secrets.Request {
	return &secrets.Request{
		Name: kubeControllerManagerSecretNameFor(clusterName),
		Config: &certutil.Config{
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName: "system:kube-controller-manager",
		},
	}
}

func kubeSchedulerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-scheduler-config", clusterName)
}

func kubeAdminSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-admin-config", clusterName)
}

func kubeControllerManagerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-controller-manager-config", clusterName)
}
