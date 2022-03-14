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

package infrastructure

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/discovery"
	"go.uber.org/multierr"
	"k8s.io/client-go/util/workqueue"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Subnets struct {
	EC2 *ec2.EC2
}

func (s *Subnets) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.Infrastructure.VPCID == nil ||
		substrate.Status.Infrastructure.PrivateRouteTableID == nil ||
		substrate.Status.Infrastructure.PublicRouteTableID == nil {
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
		if subnet == nil { // we can run into a case when ctx is canceled, errs and subnets are all nil
			continue
		}
		if aws.BoolValue(subnet.MapPublicIpOnLaunch) {
			substrate.Status.Infrastructure.PublicSubnetIDs = append(substrate.Status.Infrastructure.PublicSubnetIDs, aws.StringValue(subnet.SubnetId))
		} else {
			substrate.Status.Infrastructure.PrivateSubnetIDs = append(substrate.Status.Infrastructure.PrivateSubnetIDs, aws.StringValue(subnet.SubnetId))
		}
	}
	return reconcile.Result{}, nil
}

func (s *Subnets) ensure(ctx context.Context, substrate *v1alpha1.Substrate, subnetSpec *v1alpha1.SubnetSpec) (*ec2.Subnet, error) {
	subnet, err := s.ensureSubnet(ctx, substrate, subnetSpec)
	if err != nil {
		return nil, err
	}
	routeTableID := substrate.Status.Infrastructure.PrivateRouteTableID
	if subnetSpec.Public {
		routeTableID = substrate.Status.Infrastructure.PublicRouteTableID
	}
	if _, err := s.EC2.AssociateRouteTableWithContext(ctx, &ec2.AssociateRouteTableInput{RouteTableId: routeTableID, SubnetId: subnet.SubnetId}); err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() != "Resource.AlreadyAssociated" {
			return nil, fmt.Errorf("associating route table with subnet, %w", err)
		}
	}
	logging.FromContext(ctx).Debugf("Ensured association of route table %s to subnet %s", aws.StringValue(routeTableID), aws.StringValue(subnet.SubnetId))
	if !subnetSpec.Public {
		return subnet, nil
	}
	if _, err := s.EC2.ModifySubnetAttribute(&ec2.ModifySubnetAttributeInput{SubnetId: subnet.SubnetId, MapPublicIpOnLaunch: &ec2.AttributeBooleanValue{Value: aws.Bool(true)}}); err != nil {
		return nil, fmt.Errorf("modifying subnet attribute, %w", err)
	}
	subnet.MapPublicIpOnLaunch = aws.Bool(true)
	logging.FromContext(ctx).Debugf("Ensured subnet %s is public", aws.StringValue(subnet.SubnetId))
	return subnet, nil
}

func (s *Subnets) ensureSubnet(ctx context.Context, substrate *v1alpha1.Substrate, subnetSpec *v1alpha1.SubnetSpec) (*ec2.Subnet, error) {
	name := subnetName(substrate, subnetSpec.Zone, subnetSpec.Public)
	describeSubnetsOutput, err := s.EC2.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{Filters: discovery.Filters(substrate, name)})
	if err != nil {
		return nil, fmt.Errorf("describing subnets, %w", err)
	}
	if len(describeSubnetsOutput.Subnets) > 0 {
		logging.FromContext(ctx).Debugf("Found subnet %s", aws.StringValue(name))
		return describeSubnetsOutput.Subnets[0], nil
	}
	// tag required by ELB controller to discover these subnets to configure ELB
	createSubnetsOutput, err := s.EC2.CreateSubnetWithContext(ctx, &ec2.CreateSubnetInput{
		AvailabilityZone:  aws.String(subnetSpec.Zone),
		CidrBlock:         aws.String(subnetSpec.CIDR),
		VpcId:             substrate.Status.Infrastructure.VPCID,
		TagSpecifications: []*ec2.TagSpecification{{ResourceType: aws.String(ec2.ResourceTypeSubnet), Tags: discovery.Tags(substrate, name)}},
	})
	if err != nil {
		return nil, fmt.Errorf("creating subnet, %w", err)
	}
	logging.FromContext(ctx).Infof("Created subnet %s", aws.StringValue(name))
	return createSubnetsOutput.Subnet, nil
}

func (s *Subnets) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	routeTablesOutput, err := s.EC2.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing subnets, %w", err)
	}
	for _, routeTable := range routeTablesOutput.RouteTables {
		for _, association := range routeTable.Associations {
			if _, err := s.EC2.DisassociateRouteTableWithContext(ctx, &ec2.DisassociateRouteTableInput{AssociationId: association.RouteTableAssociationId}); err != nil {
				return reconcile.Result{}, fmt.Errorf("disassociating route table from subnet, %s", err)
			}
			logging.FromContext(ctx).Debugf("Deleted association of route table %s to subnet %s", aws.StringValue(routeTable.RouteTableId), aws.StringValue(association.SubnetId))
		}
	}
	subnetsOutput, err := s.EC2.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing subnets, %w", err)
	}
	for _, subnet := range subnetsOutput.Subnets {
		if _, err := s.EC2.DeleteSubnetWithContext(ctx, &ec2.DeleteSubnetInput{SubnetId: subnet.SubnetId}); err != nil {
			if err.(awserr.Error).Code() == "DependencyViolation" {
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, fmt.Errorf("deleting subnet, %w", err)
		}
		logging.FromContext(ctx).Infof("Deleted subnet %s", aws.StringValue(subnetsOutput.Subnets[0].SubnetId))
	}
	return reconcile.Result{}, nil
}

func subnetName(substrate *v1alpha1.Substrate, zone string, public bool) *string {
	if public {
		return discovery.Name(substrate, zone, "public")
	}
	return discovery.Name(substrate, zone, "private")
}
