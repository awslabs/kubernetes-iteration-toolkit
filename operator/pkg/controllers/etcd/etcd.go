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
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"github.com/awslabs/kit/operator/pkg/utils/common"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	certutil "k8s.io/client-go/util/cert"
)

const (
	etcdRootCACommonName = "etcd/ca"
)

type Controller struct {
	kubeClient   *kubeprovider.Client
	certificates *common.CertificatesProvider
}

type reconciler func(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error)

func New(kubeclient *kubeprovider.Client) *Controller {
	return &Controller{kubeClient: kubeclient, certificates: common.New(kubeclient)}
}

func (c *Controller) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	for _, reconcile := range []reconciler{
		c.reconcileService,
		c.reconcileSecrets,
		c.reconcileStatefulSet,
	} {
		if err := reconcile(ctx, controlPlane); err != nil {
			return err
		}
	}
	zap.S().Infof("[%v] etcd reconciled", controlPlane.ClusterName())
	return nil
}

func (c *Controller) reconcileSecrets(ctx context.Context, cp *v1alpha1.ControlPlane) error {
	// create the root CA, certs and key for etcd
	rootCA := rootCACertConfig(object.NamespacedName(caSecretNameFor(cp.ClusterName()), cp.NamespaceName()))
	secretTreeMap := common.CertTree{
		rootCA: {
			etcdServerCertConfig(cp),
			etcdPeerCertConfig(cp),
		},
	}
	return c.certificates.ReconcileFor(ctx, secretTreeMap, cp)
}

func caSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-ca", clusterName)
}

func serverSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-server", clusterName)
}

func etcdPeerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-peer", clusterName)
}

func rootCACertConfig(nn types.NamespacedName) *secrets.Request {
	return &secrets.Request{
		Name:      nn.Name,
		Namespace: nn.Namespace,
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
		Name:      serverSecretNameFor(controlPlane.ClusterName()),
		Namespace: controlPlane.NamespaceName(),
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			CommonName:   "etcd",
			Organization: []string{"kubernetes"},
			AltNames: certutil.AltNames{
				DNSNames: append(etcdPodAndHostnames(controlPlane),
					etcdSvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace),
					"localhost"),
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
		Name:      etcdPeerSecretNameFor(controlPlane.ClusterName()),
		Namespace: controlPlane.NamespaceName(),
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			CommonName:   "etcd",
			Organization: []string{"kubernetes"},
			AltNames: certutil.AltNames{
				DNSNames: append(etcdPodAndHostnames(controlPlane),
					etcdSvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace),
					"localhost"),
				IPs: []net.IP{net.IPv4(127, 0, 0, 1)},
			},
		},
	}
}

// Service name if <clustername>-etcd.<namespace>.svc.cluster.local
func etcdSvcFQDN(clusterName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", serviceNameFor(clusterName), namespace)
}

// For a given cluster name example, podnames are <clusternme>-etcd-[0-n-1], and
// hostnames are <podname>.<svcname>.kit.svc.cluster.local
func etcdPodAndHostnames(controlPlane *v1alpha1.ControlPlane) []string {
	result := []string{}
	for i := 0; i < defaultEtcdReplicas; i++ {
		podname := fmt.Sprintf("%s-etcd-%d", controlPlane.ClusterName(), i)
		result = append(result, podname, fmt.Sprintf("%s.%s", podname, etcdSvcFQDN(controlPlane.ClusterName(), controlPlane.Namespace)))
	}
	return result
}
