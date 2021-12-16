package substrate

import (
	"context"
	"fmt"

	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

func NewController(ctx context.Context) *Controller {
	return &Controller{}
}

type Controller struct {
	// dependencies
}

func (c *Controller) Reconcile(ctx context.Context, substrate *v1alpha1.Substrate) error {

}

func (c *Controller) Finalize(ctx context.Context, substrate *v1alpha1.Substrate) error {

}

func Reconcile(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// Create a VPC
	// Create subnets
	// Create elastic IP for NatGW
	// Create NatGW
	// Create routes
	// Create security groups
	// Create a launch template
	// create an ASG with this launch template
	sesssion := aws{}
	for _, resource := range []AWSResource{} {
		if err := resource.Provision(ctx, substrate); err != nil {
			return fmt.Errorf("failed to create a resource, %w", err)
		}
	}
	zap.S().Infof("Successfully created all the resources")
	// create the kubeconfig file for this substrate cluster
	return nil
}

type AWSResource interface {
	Provision(context.Context, *v1alpha1.Substrate) error
	Deprovision(context.Context, *v1alpha1.Substrate) error
}
