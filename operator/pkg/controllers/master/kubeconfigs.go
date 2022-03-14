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

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	pkiutil "github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/pki"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/kubeconfigs"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/secrets"
	"go.uber.org/zap"

	v1 "k8s.io/api/core/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
	clusterName := controlPlane.ClusterName()
	ns := controlPlane.Namespace
	for _, request := range []*kubeconfigs.Request{
		kubeConfigRequest(clusterName, ns, endpoint, kubeAdminAuthRequest(clusterName, caSecret)),
		kubeConfigRequest(clusterName, ns, localhostEndpoint, kubeSchedulerAuthRequest(clusterName, caSecret)),
		kubeConfigRequest(clusterName, ns, localhostEndpoint, kubeControllerManagerAuthRequest(clusterName, caSecret)),
	} {
		if err := c.kubeConfigs.ReconcileConfigFor(ctx, controlPlane, request); err != nil {
			return err
		}
	}
	zap.S().Debugf("[%v] Kube configs reconciled", controlPlane.ClusterName())
	return nil
}

type authRequest struct {
	config *certutil.Config
	name   string
	caCert []byte
	caKey  []byte
}

func kubeConfigRequest(clusterName, ns, endpoint string, clientAuth *authRequest) *kubeconfigs.Request {
	contextName := fmt.Sprintf("%s@%s", clientAuth.name, clusterName)
	return &kubeconfigs.Request{
		ClusterContext:    contextName,
		ApiServerEndpoint: endpoint,
		Name:              clientAuth.name,
		ClusterName:       clusterName,
		Namespace:         ns,
		AuthInfo:          clientAuth,
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: clientAuth.name,
			},
		},
	}
}

func kubeAdminAuthRequest(clusterName string, caSecret *v1.Secret) *authRequest {
	caKey, caCert := secrets.Parse(caSecret)
	return &authRequest{
		name:   KubeAdminSecretNameFor(clusterName),
		caCert: caCert,
		caKey:  caKey,
		config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   "kubernetes-admin",
			Organization: []string{"system:masters"},
		},
	}
}

func kubeSchedulerAuthRequest(clusterName string, caSecret *v1.Secret) *authRequest {
	caKey, caCert := secrets.Parse(caSecret)
	return &authRequest{
		name:   KubeSchedulerSecretNameFor(clusterName),
		caCert: caCert,
		caKey:  caKey,
		config: &certutil.Config{
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName: "system:kube-scheduler",
		},
	}
}

func kubeControllerManagerAuthRequest(clusterName string, caSecret *v1.Secret) *authRequest {
	caKey, caCert := secrets.Parse(caSecret)
	return &authRequest{
		name:   KubeControllerManagerSecretNameFor(clusterName),
		caCert: caCert,
		caKey:  caKey,
		config: &certutil.Config{
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName: "system:kube-controller-manager",
		},
	}
}

func (r *authRequest) Generate() (map[string]*clientcmdapi.AuthInfo, error) {
	private, public, err := pkiutil.GenerateSignedCertAndKey(r.config, r.caCert, r.caKey)
	if err != nil {
		return nil, err
	}
	return map[string]*clientcmdapi.AuthInfo{
		r.name: {
			ClientKeyData:         private,
			ClientCertificateData: public,
		},
	}, err
}

func (r *authRequest) CACert() []byte {
	return r.caCert
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
