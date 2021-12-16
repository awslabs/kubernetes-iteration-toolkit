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

package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"go.uber.org/multierr"
	"k8s.io/client-go/util/workqueue"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type subnets struct {
	ec2Client *ec2.EC2
}

func (s *subnets) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.VPCID == nil || substrate.Status.PrivateRouteTableID == nil || substrate.Status.PublicRouteTableID == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	subnets := make([]*ec2.Subnet, len(substrate.Spec.Subnets))
	errs := make([]error, len(substrate.Spec.Subnets))
	workqueue.ParallelizeUntil(ctx, len(substrate.Spec.Subnets), len(substrate.Spec.Subnets), func(i int) {
		subnets[i], errs[i] = s.ensure(ctx, substrate, substrate.Spec.Subnets[i])
	})
	if err := multierr.Combine(errs...); err != nil {
		return reconcile.Result{}, err
	}
	for _, subnet := range subnets {
		if aws.BoolValue(subnet.MapPublicIpOnLaunch) {
			substrate.Status.PublicSubnetIDs = append(substrate.Status.PublicSubnetIDs, aws.StringValue(subnet.SubnetId))
		} else {
			substrate.Status.PrivateSubnetIDs = append(substrate.Status.PrivateSubnetIDs, aws.StringValue(subnet.SubnetId))
		}
	}
	return reconcile.Result{}, nil
}

func (s *subnets) ensure(ctx context.Context, substrate *v1alpha1.Substrate, subnetSpec *v1alpha1.SubnetSpec) (*ec2.Subnet, error) {
	subnet, err := s.ensureSubnet(ctx, substrate, subnetName(substrate.Name, subnetSpec.Zone, subnetSpec.Public), subnetSpec.Zone, subnetSpec.CIDR)
	if err != nil {
		return nil, err
	}
	routeTableID := substrate.Status.PrivateRouteTableID
	if subnetSpec.Public {
		routeTableID = substrate.Status.PublicRouteTableID
	}
	if _, err := s.ec2Client.AssociateRouteTableWithContext(ctx, &ec2.AssociateRouteTableInput{RouteTableId: routeTableID, SubnetId: subnet.SubnetId}); err != nil {
		return nil, fmt.Errorf("associating route table with subnet, %w", err)
	}
	logging.FromContext(ctx).Infof("Ensured association of route table %s to subnet %s", aws.StringValue(routeTableID), aws.StringValue(subnet.SubnetId))
	if !subnetSpec.Public {
		return subnet, nil
	}
	if _, err := s.ec2Client.ModifySubnetAttribute(&ec2.ModifySubnetAttributeInput{SubnetId: subnet.SubnetId, MapPublicIpOnLaunch: &ec2.AttributeBooleanValue{Value: aws.Bool(true)}}); err != nil {
		return nil, fmt.Errorf("modifying subnet attribute, %w", err)
	}
	subnet.MapPublicIpOnLaunch = aws.Bool(true)
	logging.FromContext(ctx).Infof("Ensured subnet %s is public", aws.StringValue(subnet.SubnetId))
	return subnet, nil
}

func (s *subnets) ensureSubnet(ctx context.Context, substrate *v1alpha1.Substrate, name string, zone string, cidr string) (*ec2.Subnet, error) {
	describeSubnetsOutput, err := s.ec2Client.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{Filters: filtersFor(substrate.Name, name)})
	if err != nil {
		return nil, fmt.Errorf("describing subnets, %w", err)
	}
	if len(describeSubnetsOutput.Subnets) > 0 {
		logging.FromContext(ctx).Infof("Found subnet %s", name)
		return describeSubnetsOutput.Subnets[0], nil
	}
	createSubnetsOutput, err := s.ec2Client.CreateSubnetWithContext(ctx, &ec2.CreateSubnetInput{
		AvailabilityZone:  aws.String(zone),
		CidrBlock:         aws.String(cidr),
		VpcId:             substrate.Status.VPCID,
		TagSpecifications: tagsFor(ec2.ResourceTypeSubnet, substrate.Name, name),
	})
	if err != nil {
		return nil, fmt.Errorf("creating subnet, %w", err)
	}
	logging.FromContext(ctx).Infof("Created subnet %s", name)
	return createSubnetsOutput.Subnet, nil
}

func (s *subnets) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	routeTablesOutput, err := s.ec2Client.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{Filters: filtersFor(substrate.Name)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing subnets, %w", err)
	}
	for _, routeTable := range routeTablesOutput.RouteTables {
		for _, association := range routeTable.Associations {
			if _, err := s.ec2Client.DisassociateRouteTableWithContext(ctx, &ec2.DisassociateRouteTableInput{AssociationId: association.RouteTableAssociationId}); err != nil {
				return reconcile.Result{}, fmt.Errorf("disassociating route table from subnet, %s", err)
			}
			logging.FromContext(ctx).Infof("Deleted association of route table %s to subnet %s", aws.StringValue(routeTable.RouteTableId), aws.StringValue(association.SubnetId))
		}
	}
	subnetsOutput, err := s.ec2Client.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{Filters: filtersFor(substrate.Name)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing subnets, %w", err)
	}
	for _, subnet := range subnetsOutput.Subnets {
		if _, err := s.ec2Client.DeleteSubnetWithContext(ctx, &ec2.DeleteSubnetInput{SubnetId: subnet.SubnetId}); err != nil {
			if err.(awserr.Error).Code() == "DependencyViolation" {
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, fmt.Errorf("deleting subnet, %w", err)
		}
		logging.FromContext(ctx).Infof("Deleted subnet %s", aws.StringValue(subnetsOutput.Subnets[0].SubnetId))
	}
	return reconcile.Result{}, nil
}

func subnetName(idenfiier string, zone string, public bool) string {
	if public {
		return fmt.Sprintf("%s-public-%s", idenfiier, zone)
	}
	return fmt.Sprintf("%s-private-%s", idenfiier, zone)
}
