package master

import (
	"fmt"

	"github.com/awslabs/kit/operator/pkg/utils/secrets"
	v1 "k8s.io/api/core/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func KubeConfigFor(userName, clusterName, endpoint string, caSecret, secret *v1.Secret) *clientcmdapi.Config {
	contextName := fmt.Sprintf("%s@%s", userName, clusterName)
	caCert, _ := secrets.Parse(caSecret)
	cert, key := secrets.Parse(secret)
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

// func secretFor(configName, namespace string, config []byte) *v1.Secret {

// }
