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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type internetGateway struct {
	ec2api *awsprovider.EC2
}

// NewInternetGWController returns a controller for managing internet-gateway in AWS
func NewInternetGWController(ec2api *awsprovider.EC2) *internetGateway {
	return &internetGateway{ec2api: ec2api}
}

// Name returns the name of the controller
func (i *internetGateway) Name() string {
	return "internet-gateway"
}

// For returns the resource this controller is for.
func (i *internetGateway) For() controllers.Object {
	return &v1alpha1.ControlPlane{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (i *internetGateway) Reconcile(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	// 1. Get the VPC ID for the control plane
	vpcID := controlPlane.Status.Infrastructure.VPCID
	if vpcID == "" {
		return resourceReconcileFailed, fmt.Errorf("waiting to create internet-gateway, vpc does not exist")
	}
	// 2. Check if the internet gateway exists in AWS
	igw, err := i.getInternetGateway(ctx, controlPlane.Name)
	if err != nil {
		return resourceReconcileFailed, fmt.Errorf("getting internet-gateway, %w", err)
	}
	// 3. create an internet-gateway in AWS if required
	if igw == nil || aws.StringValue(igw.InternetGatewayId) == "" {
		if igw, err = i.createInternetGateway(ctx, controlPlane.Name); err != nil {
			return resourceReconcileFailed, fmt.Errorf("creating internet-gateway, %w", err)
		}
	} else {
		zap.S().Debugf("Successfully discovered internet-gateway %v for cluster %v", *igw.InternetGatewayId, controlPlane.Name)
	}
	// 4. Check igw is attached to the desired VPC ID
	if len(igw.Attachments) == 0 || *igw.Attachments[0].VpcId != vpcID {
		if err := i.attachInternetGWToVPC(ctx, *igw.InternetGatewayId, vpcID); err != nil {
			return resourceReconcileFailed, fmt.Errorf("attaching internet-gateway, %w", err)
		}
	}
	// 4. Sync resource status with the controlPlane status object in Kubernetes
	i.syncStatus(ctx, controlPlane)
	return resourceReconcileSucceeded, nil
}

// Finalize deletes the resource from AWS
func (i *internetGateway) Finalize(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	// TODO check if we need to detach from VPC before calling delete
	if err := i.deleteInternetGateway(ctx, controlPlane.Name); err != nil {
		return resourceReconcileFailed, err
	}
	i.syncStatus(ctx, controlPlane)
	return resourceReconcileSucceeded, nil
}

func (i *internetGateway) createInternetGateway(ctx context.Context, clusterName string) (*ec2.InternetGateway, error) {
	output, err := i.ec2api.CreateInternetGatewayWithContext(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: generateEC2Tags(i.Name(), clusterName),
	})
	if err != nil {
		return nil, err
	}
	zap.S().Infof("Successfully created internet-gateway %v for cluster %v", *output.InternetGateway.InternetGatewayId, clusterName)
	return output.InternetGateway, nil
}

func (i *internetGateway) attachInternetGWToVPC(ctx context.Context, igwID, vpcID string) error {
	if _, err := i.ec2api.AttachInternetGatewayWithContext(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: aws.String(igwID),
		VpcId:             aws.String(vpcID),
	}); err != nil {
		return err
	}
	zap.S().Infof("Successfully attached internet-gateway %s to VPC ID %s", igwID, vpcID)
	return nil
}

func (i *internetGateway) deleteInternetGateway(ctx context.Context, clusterName string) error {
	igw, err := i.getInternetGateway(ctx, clusterName)
	if err != nil {
		return err
	}
	if igw == nil || *igw.InternetGatewayId == "" {
		return nil
	}
	if _, err := i.ec2api.DeleteInternetGatewayWithContext(ctx, &ec2.DeleteInternetGatewayInput{
		InternetGatewayId: aws.String(*igw.InternetGatewayId),
	}); err != nil {
		return err
	}
	zap.S().Infof("Deleted internet-gateway %v for cluster %v", *igw.InternetGatewayId, clusterName)
	return nil
}

func (i *internetGateway) getInternetGateway(ctx context.Context, clusterName string) (*ec2.InternetGateway, error) {
	input := &ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String(fmt.Sprintf("tag:%s", TagKeyNameForAWSResources)),
				Values: []*string{aws.String(clusterName)},
			},
		},
	}
	output, err := i.ec2api.DescribeInternetGatewaysWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("describing internet-gateway, %w", err)
	}
	if len(output.InternetGateways) == 0 {
		return nil, nil
	}
	if len(output.InternetGateways) > 1 {
		return nil, fmt.Errorf("expected to find one internet-gateway, but found %d", len(output.InternetGateways))
	}
	return output.InternetGateways[0], nil
}

// syncStatus syncs the status for internet-gateway in AWS with the controlPlane object status in Kubernetes
func (i *internetGateway) syncStatus(ctx context.Context, controlPlane *v1alpha1.ControlPlane) {
	igw, err := i.getInternetGateway(ctx, controlPlane.Name)
	if err != nil {
		zap.S().Errorf("Failed to sync internet-gateway status, %v", err)
		return
	}
	igwID := ""
	if igw != nil {
		igwID = aws.StringValue(igw.InternetGatewayId)
	}
	controlPlane.Status.Infrastructure.InternetGatewayID = igwID
}
