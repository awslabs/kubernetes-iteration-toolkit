package substrate

import (
	"context"
	"fmt"
	"time"

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
	return nil
}

func (c *Controller) Finalize(ctx context.Context, substrate *v1alpha1.Substrate) error {
	return nil
}

func Reconcile(ctx context.Context, substrate *v1alpha1.Substrate) error {
	ec2Client := EC2Client(NewSession())
	iamClient := IAMClient(NewSession())
	start := time.Now()
	for _, resource := range []AWSResource{
		&vpc{ec2api: ec2Client},
		&elasticIP{ec2api: ec2Client},
		&internetGateway{ec2api: ec2Client},
		&subnet{ec2api: ec2Client},
		&natGateway{ec2api: ec2Client},
		&routeTable{ec2api: ec2Client},
		&routeTableAssociation{ec2api: ec2Client},
		&iamRole{iam: iamClient},
		&iamPolicy{iam: iamClient},
		&iamProfile{iam: iamClient},
	} {
		if err := resource.Provision(ctx, substrate); err != nil {
			return fmt.Errorf("failed to create resource, %w", err)
		}
	}
	zap.S().Infof("Successfully created all the resources")
	fmt.Printf("Time take to provision all resources %v\n", time.Since(start))
	// create the kubeconfig file for this substrate cluster
	return nil
}

func Finalize(ctx context.Context, substrate *v1alpha1.Substrate) error {
	ec2Client := EC2Client(NewSession())
	iamClient := IAMClient(NewSession())
	for _, resource := range []AWSResource{
		&routeTableAssociation{ec2api: ec2Client},
		&routeTable{ec2api: ec2Client},
		&natGateway{ec2api: ec2Client},
		// need to wait for NAT Gw to be deleted for the associated subnet to be cleaned up
		&subnet{ec2api: ec2Client},
		// need to wait for all public subnets to be cleaned before IGW can be cleaned up
		&internetGateway{ec2api: ec2Client},
		&elasticIP{ec2api: ec2Client},
		&vpc{ec2api: ec2Client},
		&iamProfile{iam: iamClient},
		&iamPolicy{iam: iamClient},
		&iamRole{iam: iamClient},
	} {
		if err := resource.Deprovision(ctx, substrate); err != nil {
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
