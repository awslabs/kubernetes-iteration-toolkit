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

package controlplane_test

import (
	"context"
	"testing"

	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/controllers"
	"github.com/awslabs/kit/operator/pkg/controllers/controlplane"
	"github.com/awslabs/kit/operator/pkg/controllers/etcd"
	"github.com/awslabs/kit/operator/pkg/controllers/master"
	"github.com/awslabs/kit/operator/pkg/test/environment"
	"github.com/awslabs/kit/operator/pkg/utils/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/awslabs/kit/operator/pkg/test/expectations"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	controller controllers.Controller
	kubeClient client.Client
	env        *environment.Environment
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ControlPlane")
}

var _ = BeforeSuite(func() {
	env = environment.New()
	Expect(env.Start(scheme.SubstrateCluster)).To(Succeed(), "Failed to start environment")
	kubeClient = env.Client
	controller = controlplane.NewController(kubeClient)
})

var _ = AfterSuite(func() {
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = Describe("ControlPlane", func() {
	var controlPlane *v1alpha1.ControlPlane
	BeforeEach(func() {
		controlPlane = &v1alpha1.ControlPlane{ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster",
			Namespace: "default",
		}}
	})
	AfterEach(func() {
		ExpectCleanedUp(kubeClient)
	})
	Context("Reconcilation", func() {
		Context("Objects", func() {
			It("should create all desired objects in the same namespace", func() {
				// Create the control plane cluster
				ExpectCreated(kubeClient, controlPlane)
				// Reconcile the controller for the given control plane
				ExpectReconcileWithInjectedService(context.Background(), controlPlane)
				// check master and etcd services are created
				ExpectServiceExists(kubeClient, etcd.ServiceNameFor(controlPlane.Name), controlPlane.Namespace)
				ExpectServiceExists(kubeClient, master.ServiceNameFor(controlPlane.Name), controlPlane.Namespace)
				// check etcd and master secrets are created
				for _, secretName := range []string{
					// etcd
					etcd.CASecretNameFor(controlPlane.Name),
					etcd.PeerSecretNameFor(controlPlane.Name),
					etcd.ServerSecretNameFor(controlPlane.Name),
					etcd.EtcdAPIClientSecretNameFor(controlPlane.Name),
					// master
					master.RootCASecretNameFor(controlPlane.Name),
					master.KubeAPIServerSecretNameFor(controlPlane.Name),
					master.KubeletClientSecretNameFor(controlPlane.Name),
					master.FrontProxyCASecretNameFor(controlPlane.Name),
					master.KubeFrontProxyClientSecretNameFor(controlPlane.Name),
					master.SAKeyPairSecretNameFor(controlPlane.Name),
					// kube configs
					master.KubeAdminSecretNameFor(controlPlane.Name),
					master.KubeSchedulerSecretNameFor(controlPlane.Name),
					master.KubeControllerManagerSecretNameFor(controlPlane.Name),
				} {
					ExpectSecretExists(kubeClient, secretName, controlPlane.Namespace)
				}
				// check etcd statefulset
				ExpectStatefulSetExists(kubeClient, etcd.ServiceNameFor(controlPlane.Name), controlPlane.Namespace)
				// check master deployments
				ExpectDeploymentExists(kubeClient, master.APIServerDeploymentName(controlPlane.Name), controlPlane.Namespace)
				ExpectDeploymentExists(kubeClient, master.KCMDeploymentName(controlPlane.Name), controlPlane.Namespace)
				ExpectDeploymentExists(kubeClient, master.SchedulerDeploymentName(controlPlane.Name), controlPlane.Namespace)
			})
		})
	})
})

func ExpectReconcileWithInjectedService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) {
	genController := &controllers.GenericController{Controller: controller, Client: kubeClient}
	ExpectReconcile(ctx, genController, client.ObjectKeyFromObject(controlPlane))
	// Master components expect the service has a loadbalancer provisioned for
	// certificates generation. We manually patch the status and reconcile again
	// for all the objects to be generated.
	patchControlPlaneService(ctx, controlPlane)
	ExpectReconcile(ctx, genController, client.ObjectKeyFromObject(controlPlane))
}

func patchControlPlaneService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) {
	svc := &v1.Service{}
	Expect(kubeClient.Get(ctx, types.NamespacedName{controlPlane.Namespace, master.ServiceNameFor(controlPlane.Name)}, svc)).To(Succeed())
	svc.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{{Hostname: "localhost"}}
	Expect(kubeClient.Status().Update(ctx, svc)).To(Succeed())
}
