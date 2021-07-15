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

package controller

import (
	"context"
	"fmt"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/awsprovider"
	"github.com/awslabs/kit/operator/pkg/controllers"
	"github.com/awslabs/kit/operator/pkg/status"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type controlPlane struct {
	ec2api *awsprovider.EC2
	client.Client
	etcdprovider *etcdProvider
}

// NewControlPlaneController returns a controller for managing VPCs in AWS
func NewControlPlaneController(ec2api *awsprovider.EC2, restIface client.Client) *controlPlane {
	return &controlPlane{ec2api: ec2api, Client: restIface, etcdprovider: newETCDProvider(restIface)}
}

// Name returns the name of the controller
func (c *controlPlane) Name() string {
	return "control-plane"
}

// For returns the resource this controller is for.
func (c *controlPlane) For() controllers.Object {
	return &v1alpha1.ControlPlane{}
}

type ResourceManager interface {
	Create(context.Context, *v1alpha1.ControlPlane) error
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (c *controlPlane) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	// TODO create karpenter provisioner spec for creating new nodes.
	// deploy etcd to the management cluster
	if err := c.etcdprovider.deploy(ctx, controlPlane); err != nil {
		return nil, fmt.Errorf("deploying etcd for clustername %v, %w", controlPlane.Name, err)
	}
	// TODO deploy master to management cluster
	return status.Created, nil
}

func (c *controlPlane) Finalize(_ context.Context, _ controllers.Object) (*reconcile.Result, error) {
	return status.Terminated, nil
}
