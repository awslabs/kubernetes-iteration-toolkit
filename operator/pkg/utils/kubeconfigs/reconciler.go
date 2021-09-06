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

package kubeconfigs

import (
	"context"
	"fmt"

	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"github.com/awslabs/kit/operator/pkg/utils/keypairs"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"

	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
)

type Provider struct {
	kubeClient *kubeprovider.Client
	keypairs   *keypairs.Provider
}

type ClientAuthInfo interface {
	Generate() (map[string]*clientcmdapi.AuthInfo, error)
	CACert() []byte
}

type Request struct {
	Name              string
	ClusterName       string
	Namespace         string
	ClusterContext    string
	ApiServerEndpoint string
	Contexts          map[string]*clientcmdapi.Context
	AuthInfo          ClientAuthInfo
}

func Reconciler(kubeClient *kubeprovider.Client) *Provider {
	return &Provider{kubeClient: kubeClient, keypairs: keypairs.Reconciler(kubeClient)}
}

func (p *Provider) ReconcileConfigFor(ctx context.Context, controlPlane *v1alpha1.ControlPlane, request *Request) error {
	// Check if this secret for kubeconfig exists in the api server
	_, err := p.keypairs.GetSecretFromServer(ctx, object.NamespacedName(request.Name, request.Namespace))
	if err != nil && errors.IsNotFound(err) {
		// Generate the cert and key for the user
		auth, err := request.AuthInfo.Generate()
		if err != nil {
			return fmt.Errorf("creating cert and key for %v, %w", request.Name, err)
		}
		// certs generated for clients (admin, KCM, scheduler) are stored in the kubeconfig format.
		// generate kubeconfig for this is client and convert to YAML
		configBytes, err := runtime.Encode(clientcmdlatest.Codec, kubeConfigFor(request, request.ClusterName, auth))
		if err != nil {
			return fmt.Errorf("encoding kube config object %v, %w", request.Name, err)
		}
		secret := secrets.CreateWithConfig(object.NamespacedName(request.Name, request.Namespace), configBytes)
		if controlPlane != nil {
			secret = object.WithOwner(controlPlane, secret)
		}
		// Create a secret object with config and ensure the secret object is in the api server
		if err := p.kubeClient.EnsureCreate(ctx, secret); err != nil {
			return fmt.Errorf("ensuring kube config for %v, %w", request.Name, err)
		}
		return nil
	}
	// TODO validate the existing config in the secret
	return err
}

func kubeConfigFor(request *Request, clusterName string, auth map[string]*clientcmdapi.AuthInfo) *clientcmdapi.Config {
	return &clientcmdapi.Config{
		Kind: "Config",
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   fmt.Sprintf("https://%s:443", request.ApiServerEndpoint),
				CertificateAuthorityData: request.AuthInfo.CACert(),
			},
		},
		Contexts:       request.Contexts,
		AuthInfos:      auth,
		CurrentContext: request.ClusterContext,
	}
}
