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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	pkiutil "github.com/awslabs/kit/operator/pkg/pki"
	"github.com/awslabs/kit/operator/pkg/utils/patch"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	certutil "k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	etcdRootCACommonName = "etcd/ca"
)

type Provider struct {
	kubeClient client.Client
}

func New(kubeclient client.Client) *Provider {
	return &Provider{kubeClient: kubeclient}
}

func (p *Provider) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	// generate etcd service object
	if err := p.createService(ctx, controlPlane); err != nil {
		return err
	}
	// Create ETCD certs and keys, store them as secret in the management server
	if err := p.createETCDSecrets(ctx, controlPlane); err != nil {
		return err
	}
	// create etcd stateful set
	if err := p.createStatefulset(ctx, controlPlane); err != nil {
		return err
	}
	return nil
}

func (p *Provider) createService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdServiceNameFor(controlPlane.ClusterName()),
			Namespace: controlPlane.NamespaceName(),
			Labels:    etcdLabelFor(controlPlane.ClusterName()),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: controlPlane.APIVersion,
				Name:       controlPlane.Name,
				Kind:       controlPlane.Kind,
				UID:        controlPlane.UID,
			}},
		},
	}
	result, err := controllerutil.CreateOrPatch(ctx, p.kubeClient, svc, func() error {
		// We can't update the Spec field completely as the existing svc object
		// has some defaults set by API server like `Spec.Type: ClusterIP`. If
		// we update Spec field, we will need to set these defaults as well
		// because CreateOrPatch does a reflect.DeepEqual for the existing spec
		// and with our change, calls Patch if they are not equal.
		svc.Spec.Selector = etcdLabelFor(controlPlane.ClusterName())
		svc.Spec.Ports = []v1.ServicePort{{
			Port:       2380,
			Name:       etcdServerPortNameFor(controlPlane.ClusterName()),
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 2380},
			Protocol:   "TCP",
		}, {
			Port:       2379,
			Name:       etcdClientPortNameFor(controlPlane.ClusterName()),
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 2379},
			Protocol:   "TCP",
		}}
		svc.Spec.ClusterIP = "None"
		return nil
	})
	if err != nil {
		return fmt.Errorf("creating service %s/%s, %w", svc.Namespace, svc.Name, err)
	}
	if result != controllerutil.OperationResultNone {
		zap.S().Infof("[%s] service %s %s", controlPlane.ClusterName(), svc.Name, result)
	}
	return nil
}

func (p *Provider) createETCDSecrets(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// create the root CA, certs and key for etcd
	if err := secrets.WithRootCAName(p.kubeClient,
		etcdCASecretNameFor(controlPlane.ClusterName()),
		etcdRootCACommonName).CreateSecrets(ctx, controlPlane, certListFor(controlPlane)...); err != nil {
		return err
	}
	return nil
}

func (p *Provider) createStatefulset(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdServiceNameFor(controlPlane.ClusterName()),
			Namespace: controlPlane.NamespaceName(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: controlPlane.APIVersion,
				Name:       controlPlane.Name,
				Kind:       controlPlane.Kind,
				UID:        controlPlane.UID,
			}},
		},
	}
	result, err := controllerutil.CreateOrPatch(ctx, p.kubeClient, statefulSet, func() (err error) {
		// Generate the default pod spec for the given control plane, if user has
		// provided custom config for the etcd pod spec, patch this user
		// provided config to the default spec
		etcdSpec, err := patch.PodSpec(PodSpecFor(controlPlane), controlPlane.Spec.Etcd.Spec)
		if err != nil {
			return fmt.Errorf("failed to patch pod spec, %w", err)
		}
		statefulSet.Spec = appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: etcdLabelFor(controlPlane.ClusterName()),
			},
			ServiceName: etcdServiceNameFor(controlPlane.ClusterName()),
			Replicas:    aws.Int32(defaultEtcdReplicas),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: etcdLabelFor(controlPlane.ClusterName()),
				},
				Spec: etcdSpec,
			},
		}
		return nil
	})
	if err != nil {
		return err
	}
	zap.S().Infof("[%s] statefulset %s %s", controlPlane.ClusterName(), statefulSet.Name, result)
	return nil
}

func etcdServerPortNameFor(clusterName string) string {
	return fmt.Sprintf("etcd-server-ssl-%s", clusterName)
}

func etcdClientPortNameFor(clusterName string) string {
	return fmt.Sprintf("etcd-client-ssl-%s", clusterName)
}

func etcdServiceNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd", clusterName)
}

func etcdCASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-ca", clusterName)
}

func etcdServerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-server", clusterName)
}

func etcdPeerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd-peer", clusterName)
}

func etcdLabelFor(clusterName string) map[string]string {
	return map[string]string{
		"app": etcdServiceNameFor(clusterName),
	}
}

func certListFor(controlPlane *v1alpha1.ControlPlane) []*secrets.Request {
	return []*secrets.Request{
		etcdServerCertConfig(controlPlane),
		etcdPeerCertConfig(controlPlane),
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
		Name: etcdServerSecretNameFor(controlPlane.ClusterName()),
		CertConfig: &pkiutil.CertConfig{
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
		Name: etcdPeerSecretNameFor(controlPlane.ClusterName()),
		CertConfig: &pkiutil.CertConfig{
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
		},
	}
}

// Service name if <clustername>-etcd.<namespace>.svc.cluster.local
func etcdSvcFQDN(clusterName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", etcdServiceNameFor(clusterName), namespace)
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
