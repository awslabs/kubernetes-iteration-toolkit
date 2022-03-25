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

package expectations

import (
	"context"
	"fmt"
	"time"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/errors"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	APIServerPropagationTime  = 1 * time.Second
	ReconcilerPropagationTime = 10 * time.Second
	RequestInterval           = 1 * time.Second
)

func ExpectCreated(c client.Client, objects ...client.Object) {
	for _, object := range objects {
		Expect(c.Create(context.Background(), object)).To(Succeed())
	}
}

func ExpectReconcile(ctx context.Context, resource reconcile.Reconciler, key client.ObjectKey) {
	_, err := resource.Reconcile(ctx, reconcile.Request{NamespacedName: key})
	Expect(err).ToNot(HaveOccurred())
}

func ExpectServiceExists(c client.Client, name string, namespace string) *v1.Service {
	svc := &v1.Service{}
	Expect(c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, svc)).To(Succeed())
	return svc
}

func ExpectSecretExists(c client.Client, name string, namespace string) *v1.Secret {
	secret := &v1.Secret{}
	Expect(c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, secret)).To(Succeed())
	return secret
}

func ExpectDeploymentExists(c client.Client, name string, namespace string) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	Expect(c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, deployment)).To(Succeed())
	return deployment
}

func ExpectStatefulSetExists(c client.Client, name string, namespace string) *appsv1.StatefulSet {
	set := &appsv1.StatefulSet{}
	Expect(c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, set)).To(Succeed())
	return set
}

func ExpectCleanedUp(c client.Client) {
	ctx := context.Background()
	controlPlanes := v1alpha1.ControlPlaneList{}
	Expect(c.List(ctx, &controlPlanes)).To(Succeed())
	for _, provisioner := range controlPlanes.Items {
		ExpectDeleted(c, &provisioner)
	}
	services := v1.ServiceList{}
	Expect(c.List(ctx, &services)).To(Succeed())
	for _, service := range services.Items {
		ExpectDeleted(c, &service)
	}
	secrets := v1.SecretList{}
	Expect(c.List(ctx, &secrets)).To(Succeed())
	for _, secret := range secrets.Items {
		ExpectDeleted(c, &secret)
	}
}

func ExpectDeleted(c client.Client, objects ...client.Object) {
	for _, object := range objects {
		object.SetFinalizers([]string{})
		Expect(c.Patch(context.Background(), object, client.MergeFrom(object))).To(Succeed())
		if err := c.Delete(context.Background(), object, &client.DeleteOptions{GracePeriodSeconds: ptr.Int64(0)}); !errors.IsNotFound(err) {
			Expect(err).To(BeNil())
		}
	}
	for _, object := range objects {
		ExpectNotFound(c, object)
	}
}

func ExpectNotFound(c client.Client, objects ...client.Object) {
	for _, object := range objects {
		Eventually(func() bool {
			return errors.IsNotFound(c.Get(context.Background(), types.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, object))
		}, ReconcilerPropagationTime, RequestInterval).Should(BeTrue(), func() string {
			return fmt.Sprintf("expected %s to be deleted, but it still exists", object.GetSelfLink())
		})
	}
}
