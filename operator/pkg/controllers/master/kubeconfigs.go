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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	certutil "k8s.io/client-go/util/cert"
)

const (
	localhostEndpoint = "127.0.0.1"
)

// reconcileKubeConfigs creates required kube configs and stores them as secret in api server
func (c *Controller) reconcileKubeConfigs(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// kubeconfig certs are signed by master root CA certs, if the root CA is not found return
	caSecret, err := c.keypairs.GetSecretFromServer(ctx,
		object.NamespacedName(RootCASecretNameFor(controlPlane.ClusterName()), controlPlane.Namespace))
	if err != nil {
		return err
	}
	endpoint, err := c.getClusterEndpoint(ctx, object.NamespacedName(
		controlPlane.ClusterName(), controlPlane.Namespace))
	if err != nil {
		return err
	}
	for _, request := range []*configRequest{
		kubeAdminCertConfig(controlPlane.ClusterName(), endpoint, caSecret),
		kubeSchedulerCertConfig(controlPlane.ClusterName(), localhostEndpoint, caSecret),
		kubeControllerManagerCertConfig(controlPlane.ClusterName(), localhostEndpoint, caSecret),
	} {
		if err := c.reconcileConfigFor(ctx, controlPlane, request); err != nil {
			return err
		}
	}
	zap.S().Debugf("[%v] Kube configs reconciled", controlPlane.ClusterName())
	return nil
}

func (c *Controller) reconcileConfigFor(ctx context.Context, controlPlane *v1alpha1.ControlPlane, request *configRequest) error {
	clusterName := controlPlane.ClusterName()
	namespace := controlPlane.Namespace
	// Check if this secret for kubeconfig exists in the api server
	_, err := c.keypairs.GetSecretFromServer(ctx, object.NamespacedName(request.auth.Name, namespace))
	if err != nil && errors.IsNotFound(err) {
		// Generate the cert and key for the user
		secret, err := request.auth.Create()
		if err != nil {
			return fmt.Errorf("creating cert and key for %v, %w", request.auth.CommonName, err)
		}
		// certs generated for clients (admin, KCM, scheduler) are stored in the kubeconfig format.
		// generate kubeconfig for this is client and convert to YAML
		configBytes, err := runtime.Encode(clientcmdlatest.Codec, kubeConfigFor(request, clusterName, secret))
		if err != nil {
			return fmt.Errorf("encoding kube config object %v, %w", request.auth.CommonName, err)
		}
		// Create a secret object with config and ensure the secret object is in the api server
		if err := c.kubeClient.EnsureCreate(ctx, object.WithOwner(controlPlane,
			secrets.CreateWithConfig(object.NamespacedName(request.auth.Name, namespace), configBytes)),
		); err != nil {
			return fmt.Errorf("ensuring kube config for %v, %w", request.auth.CommonName, err)
		}
		return nil
	}
	// TODO validate the existing config in the secret
	return err
}

func kubeConfigFor(request *configRequest, clusterName string, userSecret *v1.Secret) *clientcmdapi.Config {
	contextName := fmt.Sprintf("%s@%s", request.auth.Name, clusterName)
	_, caCert := secrets.Parse(request.auth.CASecret)
	key, cert := secrets.Parse(userSecret)
	return &clientcmdapi.Config{
		Kind: "Config",
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   fmt.Sprintf("https://%s:443", request.endpoint),
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: request.auth.Name,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			request.auth.Name: {
				ClientKeyData:         key,
				ClientCertificateData: cert,
			},
		},
		CurrentContext: contextName,
	}
}

type configRequest struct {
	endpoint string
	auth     *secrets.Request
}

func kubeAdminCertConfig(clusterName, endpoint string, caSecret *v1.Secret) *configRequest {
	return &configRequest{
		endpoint: endpoint,
		auth: &secrets.Request{
			Name:     KubeAdminSecretNameFor(clusterName),
			Type:     secrets.KeyWithSignedCert,
			CASecret: caSecret,
			Config: &certutil.Config{
				Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				CommonName:   "kubernetes-admin",
				Organization: []string{"system:masters"},
			},
		},
	}
}

func kubeSchedulerCertConfig(clusterName, endpoint string, caSecret *v1.Secret) *configRequest {
	return &configRequest{
		endpoint: endpoint,
		auth: &secrets.Request{
			Name:     KubeSchedulerSecretNameFor(clusterName),
			Type:     secrets.KeyWithSignedCert,
			CASecret: caSecret,
			Config: &certutil.Config{
				Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				CommonName: "system:kube-scheduler",
			},
		},
	}
}

func kubeControllerManagerCertConfig(clusterName, endpoint string, caSecret *v1.Secret) *configRequest {
	return &configRequest{
		endpoint: endpoint,
		auth: &secrets.Request{
			Name:     KubeControllerManagerSecretNameFor(clusterName),
			Type:     secrets.KeyWithSignedCert,
			CASecret: caSecret,
			Config: &certutil.Config{
				Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				CommonName: "system:kube-controller-manager",
			},
		},
	}
}

func KubeSchedulerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-scheduler-config", clusterName)
}

func KubeAdminSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-admin-config", clusterName)
}

func KubeControllerManagerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-controller-manager-config", clusterName)
}
