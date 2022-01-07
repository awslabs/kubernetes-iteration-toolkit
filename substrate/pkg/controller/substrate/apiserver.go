package substrate

import (
	"context"
	"fmt"
	"io/ioutil"
	"sync"

	pkiutil "github.com/awslabs/kit/operator/pkg/pki"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	certutil "k8s.io/client-go/util/cert"
	"knative.dev/pkg/logging"
)

const (
	kubeconfigFilename = "/tmp/kubeconfig-for-%s"
)

type KubeAPIServer struct {
	caCert []byte
	caKey  []byte
	doOnce sync.Once
}

// Create writes a kubeconfig file
func (a *KubeAPIServer) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// cacert is generated only when launch template is created and its gets added as the user-data
	if len(a.caCert) == 0 {
		return fmt.Errorf("missing CA cert for the API server")
	}
	if substrate.Status.ElasticIPForAPIServer == nil {
		return fmt.Errorf("elastic IP for API server not found for %v", substrate.Name)
	}
	return createKubeConfig(ctx, substrate.Name, *substrate.Status.ElasticIPForAPIServer, a.caCert)
}

func (a *KubeAPIServer) Delete(context.Context, *v1alpha1.Substrate) error {
	return nil
}

func (a *KubeAPIServer) GenerateCA() (err error) {
	a.doOnce.Do(func() {
		a.caKey, a.caCert, err = pkiutil.RootCA(&certutil.Config{CommonName: "k3s-server-ca"})
		if err != nil {
			return
		}
	})
	return
}

func (a *KubeAPIServer) CACert() []byte {
	return a.caCert
}

func (a *KubeAPIServer) CAKey() []byte {
	return a.caKey
}

func createKubeConfig(ctx context.Context, clusterName, serverEndpoint string, caCert []byte) error {
	configBytes, err := runtime.Encode(clientcmdlatest.Codec, kubeConfigFor(clusterName, serverEndpoint, caCert))
	if err != nil {
		return fmt.Errorf("encoding kubeconfig for %v, %w", clusterName, err)
	}
	configFile := fmt.Sprintf(kubeconfigFilename, clusterName)
	if err := ioutil.WriteFile(configFile, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write kubeconfig, %w", err)
	}
	logging.FromContext(ctx).Infof("Successfully created kubeconfig %v", configFile)
	return nil
}

func kubeConfigFor(clusterName, serverEndpoint string, caCert []byte) *clientcmdapi.Config {
	clusterName = "k3d-" + clusterName
	userName := "admin@" + clusterName
	return &clientcmdapi.Config{
		Kind: "Config",
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   fmt.Sprintf("https://%s:443", serverEndpoint),
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			clusterName: {
				Cluster:  clusterName,
				AuthInfo: userName,
			},
		},
		CurrentContext: clusterName,
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			userName: {
				Exec: &clientcmdapi.ExecConfig{
					APIVersion: "client.authentication.k8s.io/v1alpha1",
					Command:    "aws-iam-authenticator",
					Args:       []string{"token", "-i", clusterName},
				},
			},
		},
	}
}
