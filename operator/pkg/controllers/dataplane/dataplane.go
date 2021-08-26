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

	"github.com/awslabs/kit/operator/pkg/apis/dataplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/awsprovider/launchtemplate"
	"github.com/awslabs/kit/operator/pkg/controllers"
	"github.com/awslabs/kit/operator/pkg/results"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type dataplane struct {
	kubeClient     client.Client
	launchTemplate *launchtemplate.Controller
}

// NewController returns a controller for managing VPCs in AWS
func NewController(kubeClient client.Client, lt *launchtemplate.Controller) *dataplane {
	return &dataplane{kubeClient: kubeClient, launchTemplate: lt}
}

// Name returns the name of the controller
func (d *dataplane) Name() string {
	return "dataplane"
}

// For returns the resource this controller is for.
func (d *dataplane) For() controllers.Object {
	return &v1alpha1.DataPlane{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (d *dataplane) Reconcile(ctx context.Context, object controllers.Object) (res *reconcile.Result, err error) {
	dp := object.(*v1alpha1.DataPlane)
	_ = dp
	zap.S().Info("Reconciling dataplane object")
	// Get the control plane object, if not found error
	// Get CA cert from secrets, if not found error
	// Get control plane endpoint, if not found error

	// Find the VPC we are currently running inside
	// Get the subnets possible in the VPC, hardcode the subnet for now
	// Get the security group current VM is running in, hardcode the group for now
	// Create a launch template with user data
	if err := d.launchTemplate.Reconcile(ctx, dp); err != nil {
		return results.Failed, err
	}
	// Create an ASG with desired nodecount
	return results.Created, nil
}

func (d *dataplane) Finalize(_ context.Context, _ controllers.Object) (*reconcile.Result, error) {
	return results.Terminated, nil
}
