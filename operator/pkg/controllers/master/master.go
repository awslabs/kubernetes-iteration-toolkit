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
	"net"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"github.com/awslabs/kit/operator/pkg/utils/runtime"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"
	"gopkg.in/yaml.v2"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	certutil "k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	rootCACommonName       = "kubernetes"
	frontProxyCACommonName = "front-proxy-ca"
	kubeAdminName          = "kubernetes-admin"
)

type Controller struct {
	kubeClient      *kubeprovider.Client
	secretsProvider *secrets.Provider
}

func New(kubeclient client.Client) *Controller {
	return &Controller{kubeClient: kubeprovider.New(kubeclient), secretsProvider: secrets.New(kubeclient)}
}

func (c *Controller) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	// Create a service for master this will create an endpoint for the cluster

	controlPlaneHostname, err := c.controlPlaneHostname(ctx, controlPlane)
	if err != nil {
		return err
	}

	// create master CA
	if err = c.reconcileSecrets(ctx, controlPlaneHostname, controlPlane); err != nil {
		return
	}
	if err := c.createKubeConfigs(ctx, controlPlaneHostname, controlPlane); err != nil {
		return err
	}
	return nil
}

// createMasterSecrets creates the kubernetes secrets containing all the certs
// and key required to run master API server
func (c *Controller) reconcileSecrets(ctx context.Context, hostname string, controlPlane *v1alpha1.ControlPlane) error {
	caRequest := rootCAServerCertConfig(rootCASecretNameFor(controlPlane.ClusterName()), controlPlane.NamespaceName(), rootCACommonName)
	caSecret, err := c.getSecretObj(ctx, caRequest, nil)
	if err != nil {
		return err
	}
	secretObjs := []*v1.Secret{caSecret}
	for _, request := range []*secrets.Request{
		kubeAPIServerCertConfig(hostname, controlPlane.ClusterName()),
		kubeletClientCertConfig(controlPlane.ClusterName()),
	} {
		secretObj, err := c.getSecretObj(ctx, request, caSecret)
		if err != nil {
			return err
		}
		secretObjs = append(secretObjs, secretObj)
	}
	for _, secret := range secretObjs {
		if err = controllerutil.SetOwnerReference(controlPlane, secret, runtime.Scheme()); err != nil {
			return err
		}
		if err = c.kubeClient.Ensure(ctx, secret); err != nil {
			return err
		}
	}
	return nil
}

// getSecretObj will check with API server for this object.
// Calls getSecretFromServer to get from API server and validate
// If the object is not found, it will create and return a new secret object.
func (c *Controller) getSecretObj(ctx context.Context, request *secrets.Request, caSecret *v1.Secret) (*v1.Secret, error) {
	// get secret from api server
	secret, err := c.getSecretFromServer(ctx, request.Name, request.Namespace)
	if err != nil && errors.IsNotFound(err) {
		return secrets.CreateWithCerts(request, caSecret)
	}
	return secret, err
}

// getSecretFromServer will get the secret from API server and validate
func (c *Controller) getSecretFromServer(ctx context.Context, name, namespace string) (*v1.Secret, error) {
	// get secret from api server
	secretObj := &v1.Secret{}
	if err := c.kubeClient.Get(ctx, types.NamespacedName{name, namespace}, secretObj); err != nil {
		return nil, err
	}
	// validate the secret object contains valid secret data
	err := secrets.IsValid(secretObj)
	if err != nil {
		return nil, fmt.Errorf("invalid secret object %v/%v, %w", namespace, name, err)
	}
	return secretObj, nil
}

func (c *Controller) controlPlaneHostname(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (string, error) {
	hostname, err := c.createService(ctx, controlPlane)
	if err != nil {
		return "", err
	}
	if hostname == "" {
		return "", fmt.Errorf("waiting for control plane hostname, %w", errors.WaitingForSubResources)
	}
	return hostname, nil
}

func (c *Controller) createService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (string, error) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceNameFor(controlPlane.ClusterName()),
			Namespace: controlPlane.NamespaceName(),
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-scheme":                  "internet-facing",
				"service.beta.kubernetes.io/aws-load-balancer-type":                    "nlb-ip",
				"service.beta.kubernetes.io/aws-load-balancer-target-group-attributes": "stickiness.enabled=true,stickiness.type=source_ip",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: controlPlane.APIVersion,
				Name:       controlPlane.Name,
				Kind:       controlPlane.Kind,
				UID:        controlPlane.UID,
			}},
		},
	}
	hostname := ""
	result, err := controllerutil.CreateOrPatch(ctx, c.kubeClient, svc, func() error {
		svc.Spec.Selector = labelsFor(controlPlane.ClusterName())
		svc.Spec.Ports = []v1.ServicePort{{
			Port:       443,
			Name:       apiserverPortName(controlPlane.ClusterName()),
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 443},
			Protocol:   "TCP",
		}}
		svc.Spec.Type = v1.ServiceTypeLoadBalancer
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			hostname = svc.Status.LoadBalancer.Ingress[0].Hostname
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("creating service %s/%s, %w", svc.Namespace, svc.Name, err)
	}
	if result != controllerutil.OperationResultNone {
		zap.S().Infof("[%s] service %s %s", controlPlane.ClusterName(), svc.Name, result)
	}
	return hostname, nil
}

func (c *Controller) createKubeConfigs(ctx context.Context, hostname string, controlPlane *v1alpha1.ControlPlane) error {
	clusterName := controlPlane.ClusterName()
	namespace := controlPlane.NamespaceName()
	caSecret, err := c.getSecretFromServer(ctx, rootCASecretNameFor(clusterName), namespace)
	if err != nil {
		return err
	}
	for _, request := range []*secrets.Request{
		kubeAdminCertConfig(clusterName),
		kubeSchedulerCertConfig(clusterName),
		kubeControllerManagerCertConfig(clusterName),
	} {
		// These kubeconfigs are stored in the form of secrets in the api server
		secretObj, err := c.getSecretObj(ctx, request, caSecret)
		if err != nil {
			return err
		}
		// generate kubeconfig for this is component and convert to YAML
		configBytes, err := yaml.Marshal(KubeConfigFor(request.CommonName, clusterName, hostname, caSecret, secretObj))
		if err != nil {
			return err
		}
		if err := c.kubeClient.Ensure(ctx, secrets.CreateWithConfig(
			fmt.Sprintf("%s-%s-config", clusterName, request.CommonName), namespace, configBytes)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) getObject(ctx context.Context, obj client.Object) (client.Object, error) {
	if err := c.kubeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// func certListFor(hostname, namespace, clusterName string) []*secrets.Request {
// 	return []*secrets.Request{
// 		// rootCAServerCertConfig(rootCASecretNameFor(clusterName), namespace, rootCACommonName),
// 		kubeAPIServerCertConfig(hostname, clusterName),
// 		kubeletClientCertConfig(clusterName),
// 		// kubeAdminCertConfig(clusterName),
// 		// kubeSchedulerCertConfig(clusterName),
// 		// kubeControllerManagerCertConfig(clusterName),
// 	}
// }

func rootCAServerCertConfig(name, namespace, commonName string) *secrets.Request {
	return &secrets.Request{
		Name:      name,
		Namespace: namespace,
		Config: &certutil.Config{
			CommonName: commonName,
		},
	}
}

func kubeAPIServerCertConfig(hostname, clusterName string) *secrets.Request {
	return &secrets.Request{
		Name: kubeAPIServerSecretNameFor(clusterName),
		Config: &certutil.Config{
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			CommonName: "kube-apiserver",
			AltNames: certutil.AltNames{
				DNSNames: []string{hostname, "localhost", "kubernetes", "kubernetes.default",
					"kubernetes.default.svc", "kubernetes.default.svc.cluster.local"},
				IPs: []net.IP{net.IPv4(127, 0, 0, 1), apiServerVirtualIP()},
			},
		},
	}
}

// Certificate used by the API server to connect to the kubelet
func kubeletClientCertConfig(clusterName string) *secrets.Request {
	return &secrets.Request{
		Name: kubeletClientSecretNameFor(clusterName),
		Config: &certutil.Config{
			Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName:   "kube-apiserver-kubelet-client",
			Organization: []string{"system:masters"},
		},
	}
}

// Cert used by the API server to access the front proxy.
func kubeFrontProxyClient(clusterName string) *secrets.Request {
	return &secrets.Request{
		Name: kubeFrontProxyClientSecretNameFor(clusterName),
		Config: &certutil.Config{
			Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			CommonName: "front-proxy-client",
		},
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

// TODO get this from controlPlane object
func apiServerVirtualIP() net.IP {
	return net.IPv4(10, 96, 0, 1)
}

func rootCASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-master-ca", clusterName)
}

func kubeAPIServerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-apiserver", clusterName)
}

func kubeletClientSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-apiserver-kubelet-client", clusterName)
}

func kubeFrontProxyClientSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-front-proxy-client", clusterName)
}

func kubeFrontProxyCASecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-front-proxy-ca", clusterName)
}

func kubeAdminSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-admin", clusterName)
}

func kubeSchedulerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-scheduler", clusterName)
}

func kubeControllerManagerSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-kube-controller-manager", clusterName)
}

func serviceNameFor(clusterName string) string {
	return fmt.Sprintf("%s-controlplane-endpoint", clusterName)
}

func labelsFor(clusterName string) map[string]string {
	return map[string]string{
		"app": serviceNameFor(clusterName),
	}
}

func apiserverPortName(clusterName string) string {
	return fmt.Sprintf("%s-port", serviceNameFor(clusterName))
}
