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

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/controllers"
	"github.com/awslabs/kit/operator/pkg/controllers/controlplane"
	"github.com/awslabs/kit/operator/pkg/controllers/etcd"
	"github.com/awslabs/kit/operator/pkg/controllers/master"
	"github.com/awslabs/kit/operator/pkg/test/environment"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/awslabs/kit/operator/pkg/test/expectations"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	controller controllers.Controller
	kubeClient client.Client
	env        *environment.Environment
	scheme     = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ControlPlane")
}

var _ = BeforeSuite(func() {
	env = environment.New()
	Expect(env.Start(scheme)).To(Succeed(), "Failed to start environment")
	kubeClient = env.Client
	controller = controlplane.NewController(kubeClient)
})

var _ = AfterSuite(func() {
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = Describe("Allocation", func() {
	var controlPlane *v1alpha1.ControlPlane
	BeforeEach(func() {
		controlPlane = &v1alpha1.ControlPlane{ObjectMeta: metav1.ObjectMeta{
			Name:      "testcluster",
			Namespace: "default",
		}}
	})
	AfterEach(func() {
		ExpectCleanedUP(kubeClient)
	})
	Context("Reconcilation", func() {
		Context("Objects", func() {
			It("should create all desired objects in the same namespace", func() {
				// Create the control plane cluster
				ExpectCreated(kubeClient, controlPlane)
				// Reconcile the controller for the given control plane
				genController := &controllers.GenericController{Controller: controller, Client: kubeClient}
				ExpectReconcile(context.Background(), genController, client.ObjectKeyFromObject(controlPlane))
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
