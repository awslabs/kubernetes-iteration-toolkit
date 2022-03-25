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

package dataplane

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws/session"
	cpv1alpha1 "github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/dataplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/awsprovider/instances"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/awsprovider/launchtemplate"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/kubeprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/results"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type dataplane struct {
	kubeClient     *kubeprovider.Client
	launchTemplate *launchtemplate.Controller
	instances      *instances.Controller
}

// NewController returns a controller for managing VPCs in AWS
func NewController(kubeClient client.Client, session *session.Session) *dataplane {
	return &dataplane{kubeClient: kubeprovider.New(kubeClient),
		launchTemplate: launchtemplate.NewController(
			awsprovider.EC2Client(session),
			awsprovider.SSMClient(session),
			kubeprovider.New(kubeClient),
		),
		instances: instances.NewController(awsprovider.EC2Client(session),
			awsprovider.AutoScalingClient(session),
			kubeprovider.New(kubeClient),
		),
	}
}

// Name returns the name of the controller
func (d *dataplane) Name() string {
	return "dataplane"
}

// For returns the resource this controller is for.
func (d *dataplane) For() controllers.Object {
	return &v1alpha1.DataPlane{}
}

type reconciler func(context.Context, *v1alpha1.DataPlane) (err error)

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (d *dataplane) Reconcile(ctx context.Context, object controllers.Object) (res *reconcile.Result, err error) {
	dp := object.(*v1alpha1.DataPlane)
	// Get the control plane object, if not found error, add Owner reference to control plane object
	if err := d.setOwnerForDataplane(ctx, dp); err != nil {
		return results.Failed, fmt.Errorf("setting owner reference for dataplane, %w", err)
	}
	// Create a launch template and ASG with desired node count
	for _, reconciler := range []reconciler{
		d.launchTemplate.Reconcile,
		d.instances.Reconcile,
	} {
		if err := reconciler(ctx, dp); err != nil {
			return results.Failed, err
		}
	}
	zap.S().Infof("[%s] data plane reconciled", dp.Spec.ClusterName)
	return results.Created, nil
}

func (d *dataplane) setOwnerForDataplane(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	if len(dataplane.GetOwnerReferences()) == 0 {
		cp := &cpv1alpha1.ControlPlane{}
		if err := d.kubeClient.Get(ctx, types.NamespacedName{Namespace: dataplane.GetNamespace(), Name: dataplane.Spec.ClusterName}, cp); err != nil {
			return fmt.Errorf("getting control plane object, %w", err)
		}
		return d.kubeClient.Update(ctx, object.WithOwner(cp, dataplane))
	}
	return nil
}

func (d *dataplane) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	dp := object.(*v1alpha1.DataPlane)
	for _, reconciler := range []reconciler{
		d.launchTemplate.Finalize,
		d.instances.Finalize,
	} {
		if err := reconciler(ctx, dp); err != nil {
			return results.Failed, err
		}
	}
	return results.Terminated, nil
}
