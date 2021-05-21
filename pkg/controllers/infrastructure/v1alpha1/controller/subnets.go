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
	"github.com/prateekgogia/kit/pkg/errors"
	"github.com/prateekgogia/kit/pkg/status"

	"github.com/aws/aws-sdk-go/aws"
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
	return &v1alpha1.Subnet{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the Subnet.Status
// object
func (s *subnet) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	subnetObj := object.(*v1alpha1.Subnet)
	clusterName := subnetObj.Name
	// 1. Get the VPC ID for the control plane status
	vpc, err := getVPC(ctx, s.ec2api, clusterName)
	if err != nil {
		return status.Waiting, fmt.Errorf("getting VPC %w", err)
	}
	if vpc == nil {
		return status.Waiting, fmt.Errorf("vpc does not exist %w", errors.WaitingForSubResources)
	}
	// 2. Check if subnets exists in AWS
	subnets, err := s.getSubnets(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("getting subnets from AWS, %w", err)
	}
	subnetObj.Status.PublicSubnets = nil
	subnetObj.Status.PrivateSubnets = nil
	for _, subnet := range subnetObj.Spec.Items {
		subnetID := ""
		sub, exists := contains(subnets, subnet, clusterName)
		if !exists {
			// 3. Create subnets in AWS
			subnetID, err = s.createSubnet(ctx, clusterName, *vpc.VpcId, subnet)
			if err != nil {
				return nil, fmt.Errorf("creating public subnet, %w", err)
			}
		} else {
			subnetID = *sub.SubnetId
			zap.S().Debugf("Successfully discovered subnet %s for cluster %s", *sub.SubnetId, clusterName)
		}
		if subnet.Public {
			subnetObj.Status.PublicSubnets = append(subnetObj.Status.PublicSubnets, subnetID)
		} else {
			subnetObj.Status.PrivateSubnets = append(subnetObj.Status.PrivateSubnets, subnetID)
		}
		zap.S().Infof("Created subnet ID: %s, cidr: %s, public: %v, az: %s", subnetID, subnet.CIDR, subnet.Public, subnet.AZ)
	}
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (s *subnet) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	subnetObj := object.(*v1alpha1.Subnet)
	subnets, err := s.getSubnets(ctx, subnetObj.Name)
	if err != nil {
		return nil, err
	}
	if len(subnets) != 0 {
		if err := s.deleteSubnets(ctx, subnets); err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully deleted Subnets for cluster %v", subnetObj.Name)
	}
	subnetObj.Status.PublicSubnets = subnetObj.Status.PublicSubnets[:0]
	subnetObj.Status.PrivateSubnets = subnetObj.Status.PrivateSubnets[:0]
	return status.Terminated, nil
}

func (s *subnet) createSubnet(ctx context.Context, clusterName, vpcID string, subnet *v1alpha1.SubnetProperty) (string, error) {
	input := &ec2.CreateSubnetInput{
		AvailabilityZone:  aws.String(subnet.AZ),
		CidrBlock:         aws.String(subnet.CIDR),
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2Tags(s.Name(), clusterName),
	}
	output, err := s.ec2api.CreateSubnetWithContext(ctx, input)
	if err != nil {
		if errors.IsSubnetExists(err) {
			return "", nil
		}
		return "", fmt.Errorf("creating subnet, %w", err)
	}
	if subnet.Public {
		attributeInput := &ec2.ModifySubnetAttributeInput{
			MapPublicIpOnLaunch: &ec2.AttributeBooleanValue{
				Value: aws.Bool(subnet.Public),
			},
			SubnetId: output.Subnet.SubnetId,
		}
		if _, err = s.ec2api.ModifySubnetAttribute(attributeInput); err != nil {
			return "", fmt.Errorf("modifying subnet attributes, %w", err)
		}
	}
	return *output.Subnet.SubnetId, nil
}

func (s *subnet) deleteSubnets(ctx context.Context, subnets []*ec2.Subnet) error {
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
	return getSubnets(ctx, s.ec2api, clusterName)
}

func getSubnets(ctx context.Context, ec2api *awsprovider.EC2, clusterName string) ([]*ec2.Subnet, error) {
	input := &ec2.DescribeSubnetsInput{
		Filters: ec2FilterFor(clusterName),
	}
	output, err := ec2api.DescribeSubnetsWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("describing subnet, %w", err)
	}
	return output.Subnets, nil
}

func contains(subnets []*ec2.Subnet, desired *v1alpha1.SubnetProperty, clusterName string) (*ec2.Subnet, bool) {
	for _, subnet := range subnets {
		if aws.StringValue(subnet.CidrBlock) == desired.CIDR &&
			aws.StringValue(subnet.AvailabilityZone) == desired.AZ &&
			aws.BoolValue(subnet.MapPublicIpOnLaunch) == desired.Public {
			zap.S().Debugf("Successfully discovered Subnet ID %s public %v, for cluster %s",
				*subnet.SubnetId, *subnet.MapPublicIpOnLaunch, clusterName)
			return subnet, true
		}
	}
	return nil, false
}
