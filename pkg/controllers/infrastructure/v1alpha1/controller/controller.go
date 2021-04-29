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
	"time"

	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/aws"
	"github.com/prateekgogia/kit/pkg/controllers"
)

// Controller for the resource
type Controller struct {
	ec2api *aws.EC2
}

// For returns the resource this controller is for.
func (c *Controller) For() controllers.Object {
	return &v1alpha1.Cluster{}
}

// Owns returns the resources owned by this controller's resource.
func (c *Controller) Owns() []controllers.Object {
	return []controllers.Object{}
}

func (c *Controller) Interval() time.Duration {
	return 5 * time.Second
}

func (c *Controller) Name() string {
	return "cluster/manager"
}

// NewController constructs a controller instance
func NewController(ec2api *aws.EC2) *Controller {
	return &Controller{
		ec2api: ec2api,
	}
}

// Reconcile executes an allocation control loop for the resource
func (c *Controller) Reconcile(ctx context.Context, object controllers.Object) error {
	_ = object.(*v1alpha1.Cluster)
	return nil
}
