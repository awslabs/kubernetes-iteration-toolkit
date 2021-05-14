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
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type subnet struct {
	ec2api *awsprovider.EC2
}

// NewSubnetController returns a controller for managing subnets in AWS
func NewSubnetController(ec2api *awsprovider.EC2) *subnet {
	return &subnet{ec2api: ec2api}
}

// Name returns the name of the controller
func (s *subnet) Name() string {
	return "subnet"
}

// For returns the object managed by this controller
func (s *subnet) For() controllers.Object {
	return &v1alpha1.ControlPlane{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (s *subnet) Reconcile(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	// 1. Get the VPC ID for the control plane status
	vpcID := controlPlane.Status.Infrastructure.VPCID
	if vpcID == "" {
		return resourceReconcileFailed, fmt.Errorf("waiting to create subnets, vpc does not exist")
	}
	// 2. Check if subnets exists in AWS
	subnets, err := s.getSubnets(ctx, controlPlane.Name)
	if err != nil {
		return resourceReconcileFailed, fmt.Errorf("getting subnets from AWS, %w", err)
	}
	// In future we might want to check the CIDRs for subnets to verify
	expectedCount := len(s.availabilityZonesForRegion(*s.ec2api.Config.Region)) * 2
	// 3. If not as desired, create a private and a public subnet in all azs supported
	if len(subnets) != expectedCount {
		for i, az := range s.availabilityZonesForRegion(*s.ec2api.Config.Region) {
			// create a private subnet
			if err := s.createSubnet(ctx, false, controlPlane.Name, az, vpcID, privateSubnetCIDRs[i]); err != nil {
				return resourceReconcileFailed, fmt.Errorf("creating private subnet, %w", err)
			}
			// create a public subnet
			if err := s.createSubnet(ctx, true, controlPlane.Name, az, vpcID, publicSubnetCIDRs[i]); err != nil {
				return resourceReconcileFailed, fmt.Errorf("creating public subnet, %w", err)
			}
		}
	} else {
		zap.S().Debugf("Successfully discovered Subnets for cluster %v", controlPlane.Name)
	}
	// 4. Sync resource status with the controlPlane status object in Kubernetes
	s.syncStatus(ctx, controlPlane)
	return resourceReconcileSucceeded, nil
}

// Finalize deletes the resource from AWS
func (s *subnet) Finalize(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	if err := s.deleteSubnets(ctx, controlPlane.Name); err != nil {
		return resourceReconcileFailed, err
	}
	s.syncStatus(ctx, controlPlane)
	return resourceReconcileSucceeded, nil
}

func (s *subnet) createSubnet(ctx context.Context, public bool, clusterName, az, vpcID, cidr string) error {
	input := &ec2.CreateSubnetInput{
		AvailabilityZone:  aws.String(az),
		CidrBlock:         aws.String(cidr),
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2Tags(s.Name(), clusterName),
	}
	output, err := s.ec2api.CreateSubnetWithContext(ctx, input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "InvalidSubnet.Conflict" {
				return nil
			}
		}
		return fmt.Errorf("creating subnet, %w", err)
	}
	if public {
		attributeInput := &ec2.ModifySubnetAttributeInput{
			MapPublicIpOnLaunch: &ec2.AttributeBooleanValue{
				Value: aws.Bool(public),
			},
			SubnetId: output.Subnet.SubnetId,
		}
		if _, err = s.ec2api.ModifySubnetAttribute(attributeInput); err != nil {
			return fmt.Errorf("modifying subnet attributes, %w", err)
		}
	}
	zap.S().Infof("Created subnet ID: %s, cidr: %s, public: %v, az: %s", *output.Subnet.SubnetId, cidr, public, az)
	return nil
}

// TODO get all AZs for a region from an API
func (s *subnet) availabilityZonesForRegion(region string) []string {
	azs := []string{}
	for _, azPrefix := range []string{"a", "b", "c"} {
		azs = append(azs, fmt.Sprintf(region+azPrefix))
	}
	return azs
}

func (s *subnet) deleteSubnets(ctx context.Context, clusterName string) error {
	subnets, err := s.getSubnets(ctx, clusterName)
	if err != nil {
		return nil
	}
	for _, subnet := range subnets {
		if _, err := s.ec2api.DeleteSubnetWithContext(ctx, &ec2.DeleteSubnetInput{
			SubnetId: subnet.SubnetId,
		}); err != nil {
			return fmt.Errorf("deleting subnet, %w", err)
		}
	}
	return nil
}

// getSubnets from AWS for the control plane
func (s *subnet) getSubnets(ctx context.Context, clusterName string) ([]*ec2.Subnet, error) {
	input := &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String(fmt.Sprintf("tag:%s", TagKeyNameForAWSResources)),
				Values: []*string{aws.String(clusterName)},
			},
		},
	}
	output, err := s.ec2api.DescribeSubnetsWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("describing subnet, %w", err)
	}
	return output.Subnets, nil
}

// syncStatus syncs the status for subnet in AWS with the controlPlane status object in Kubernetes
func (s *subnet) syncStatus(ctx context.Context, controlPlane *v1alpha1.ControlPlane) {
	subnets, err := s.getSubnets(ctx, controlPlane.Name)
	if err != nil {
		zap.S().Errorf("Failed to sync subnet status, %v", err)
		return
	}
	controlPlane.Status.Infrastructure.PrivateSubnets = controlPlane.Status.Infrastructure.PrivateSubnets[:0]
	controlPlane.Status.Infrastructure.PublicSubnets = controlPlane.Status.Infrastructure.PublicSubnets[:0]
	for _, subnet := range subnets {
		if aws.BoolValue(subnet.MapPublicIpOnLaunch) {
			controlPlane.Status.Infrastructure.PublicSubnets = append(controlPlane.Status.Infrastructure.PublicSubnets, *subnet.SubnetId)
			continue
		}
		controlPlane.Status.Infrastructure.PrivateSubnets = append(controlPlane.Status.Infrastructure.PrivateSubnets, *subnet.SubnetId)
	}
}
