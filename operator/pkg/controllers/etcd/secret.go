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

package etcd

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/utils/keypairs"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"

	"k8s.io/apimachinery/pkg/types"
	certutil "k8s.io/client-go/util/cert"
)

func (c *Controller) reconcileSecrets(ctx context.Context, cp *v1alpha1.ControlPlane) error {
	// create the root CA, certs and key for etcd
	rootCA := rootCACertConfig(object.NamespacedName(CASecretNameFor(cp.ClusterName()), cp.Namespace))
	secretTreeMap := keypairs.CertTree{
		rootCA: {
			etcdServerCertConfig(cp),
			etcdPeerCertConfig(cp),
			etcdAPIClientCertConfig(cp),
		},
	}
	return c.keypairs.ReconcileCertsFor(ctx, cp, secretTreeMap)
}

func CASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-ca", clusterName)
}

func ServerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-server", clusterName)
}

func PeerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-peer", clusterName)
}

func EtcdAPIClientSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-apiserver-etcd-client", clusterName)
}

func rootCACertConfig(nn types.NamespacedName) *secrets.Request {
	return &secrets.Request{
		Name:      nn.Name,
		Namespace: nn.Namespace,
		Type:      secrets.CA,
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
func etcdServerCertConfig(controlPlane *v1alpha1.ControlPlane) *secrets.Request {
	return &secrets.Request{
		Name:      ServerSecretNameFor(controlPlane.ClusterName()),
		Namespace: controlPlane.Namespace,
		Type:      secrets.KeyWithSignedCert,
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			CommonName:   "etcd",
			Organization: []string{"kubernetes"},
			AltNames: certutil.AltNames{
				DNSNames: append(etcdPodAndHostnames(controlPlane),
					SvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace), "localhost"),
				IPs: []net.IP{net.IPv4(127, 0, 0, 1)},
			},
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
func etcdPeerCertConfig(controlPlane *v1alpha1.ControlPlane) *secrets.Request {
	return &secrets.Request{
		Name:      PeerSecretNameFor(controlPlane.ClusterName()),
		Namespace: controlPlane.Namespace,
		Type:      secrets.KeyWithSignedCert,
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			CommonName:   "etcd",
			Organization: []string{"kubernetes"},
			AltNames: certutil.AltNames{
				DNSNames: append(etcdPodAndHostnames(controlPlane),
					SvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace), "localhost"),
				IPs: []net.IP{net.IPv4(127, 0, 0, 1)},
			},
		},
	}
}

func etcdAPIClientCertConfig(controlPlane *v1alpha1.ControlPlane) *secrets.Request {
	return &secrets.Request{
		Name:      EtcdAPIClientSecretNameFor(controlPlane.ClusterName()),
		Namespace: controlPlane.Namespace,
		Type:      secrets.KeyWithSignedCert,
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   "kube-apiserver-etcd-client",
			Organization: []string{"system:masters"},
		},
	}
}

// Service name if <clustername>-etcd.<namespace>.svc.cluster.local
func SvcFQDN(clusterName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", ServiceNameFor(clusterName), namespace)
}

// For a given cluster name example, podnames are <clusternme>-etcd-[0-n-1], and
// hostnames are <podname>.<svcname>.kit.svc.cluster.local
func etcdPodAndHostnames(controlPlane *v1alpha1.ControlPlane) []string {
	result := []string{}
	for i := 0; i < defaultEtcdReplicas; i++ {
		podname := fmt.Sprintf("%s-etcd-%d", controlPlane.ClusterName(), i)
		result = append(result, podname, fmt.Sprintf("%s.%s", podname, SvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace)))
	}
	return result
}
