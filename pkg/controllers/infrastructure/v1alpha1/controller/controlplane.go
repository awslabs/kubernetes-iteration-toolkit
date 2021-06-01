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

	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"github.com/prateekgogia/kit/pkg/resource"
	"github.com/prateekgogia/kit/pkg/status"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type controlPlane struct {
	ec2api *awsprovider.EC2
	client.Client
}

// NewControlPlaneController returns a controller for managing VPCs in AWS
func NewControlPlaneController(ec2api *awsprovider.EC2, restIface client.Client) *controlPlane {
	return &controlPlane{ec2api: ec2api, Client: restIface}
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
	resources := []ResourceManager{
		&resource.VPC{KubeClient: c.Client},
		&resource.Subnet{KubeClient: c.Client, Region: *c.ec2api.Config.Region},
		&resource.InternetGateway{KubeClient: c.Client},
		&resource.ElasticIP{KubeClient: c.Client},
		&resource.NatGateway{KubeClient: c.Client},
		&resource.RouteTable{KubeClient: c.Client},
		&resource.SecurityGroup{KubeClient: c.Client},
	}
	for _, resource := range resources {
		if err := resource.Create(ctx, controlPlane); err != nil {
			return nil, fmt.Errorf("creating resources %v", err)
		}
	}
	return status.Created, nil
}

func (c *controlPlane) Finalize(_ context.Context, _ controllers.Object) (*reconcile.Result, error) {
	return status.Terminated, nil
}
