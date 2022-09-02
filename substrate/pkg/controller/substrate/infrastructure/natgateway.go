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
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type NatGateway struct {
	EC2 *ec2.EC2
}

func (n *NatGateway) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.Infrastructure.VPCID == nil ||
		substrate.Status.Infrastructure.PrivateRouteTableID == nil ||
		len(substrate.Status.Infrastructure.PublicSubnetIDs) == 0 ||
		substrate.Status.Infrastructure.ElasticIpIDForNatGW == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	natGW, err := n.ensure(ctx, substrate)
	if err != nil {
		return reconcile.Result{}, err
	}
	if _, err := n.EC2.CreateRouteWithContext(ctx, &ec2.CreateRouteInput{
		RouteTableId:         substrate.Status.Infrastructure.PrivateRouteTableID,
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		NatGatewayId:         natGW.NatGatewayId,
	}); err != nil {
		if err.(awserr.Error).Code() != "RouteAlreadyExists" {
			return reconcile.Result{}, fmt.Errorf("creating route for NAT gateway, %w", err)
		}
	} else {
		logging.FromContext(ctx).Debugf("Ensured route for NAT gateway %s", aws.StringValue(natGW.NatGatewayId))
	}
	return reconcile.Result{}, nil
}

func (n *NatGateway) getNatGateway(ctx context.Context, substrate *v1alpha1.Substrate) (*ec2.NatGateway, error) {
	output, err := n.EC2.DescribeNatGatewaysWithContext(ctx, &ec2.DescribeNatGatewaysInput{Filter: discovery.Filters(substrate.Name, discovery.Name(substrate))})
	if err != nil {
		return nil, fmt.Errorf("describing nat-gateway, %w", err)
	}
	if len(output.NatGateways) == 0 {
		return nil, nil
	}
	var result *ec2.NatGateway
	for _, natgw := range output.NatGateways {
		if aws.StringValue(natgw.State) == "deleting" || aws.StringValue(natgw.State) == "deleted" ||
			aws.StringValue(natgw.State) == "failed" {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("expected to find one nat-gateway, but found %d", len(output.NatGateways))
		}
		result = natgw
	}
	return result, nil
}

func (n *NatGateway) ensure(ctx context.Context, substrate *v1alpha1.Substrate) (*ec2.NatGateway, error) {
	natGW, err := n.getNatGateway(ctx, substrate)
	if err != nil {
		return nil, fmt.Errorf("getting existing NAT GW, %w", err)
	}
	if natGW != nil {
		logging.FromContext(ctx).Debugf("Found NAT gateway %s", substrate.Name)
		return natGW, nil
	}
	output, err := n.EC2.CreateNatGatewayWithContext(ctx, &ec2.CreateNatGatewayInput{
		AllocationId: substrate.Status.Infrastructure.ElasticIpIDForNatGW,
		SubnetId:     aws.String(substrate.Status.Infrastructure.PublicSubnetIDs[0]),
		TagSpecifications: []*ec2.TagSpecification{{
			ResourceType: aws.String(ec2.ResourceTypeNatgateway),
			Tags:         discovery.Tags(substrate.Name, discovery.Name(substrate)),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("creating NAT GW, %w", err)
	}
	logging.FromContext(ctx).Infof("Created NAT gateway %s, ID %s", substrate.Name, *output.NatGateway.NatGatewayId)
	// Wait for the NAT Gateway to be available
	// There are scenarios where after creating a NAT gateway, describe NAT GW
	// call doesn't return the NatGateway ID we just created. In such cases, we
	// end up creating multiple gateways, in the end only one becomes available
	// and others end up in the failed state.
	logging.FromContext(ctx).Debugf("Waiting for NAT gateway %v to be available", substrate.Name)
	if err := n.EC2.WaitUntilNatGatewayAvailableWithContext(ctx, &ec2.DescribeNatGatewaysInput{NatGatewayIds: []*string{output.NatGateway.NatGatewayId}}); err != nil {
		return nil, fmt.Errorf("waiting for NAT GW to be ready, %w", err)
	}
	logging.FromContext(ctx).Infof("NAT gateway is available %s", substrate.Name)
	return output.NatGateway, nil
}

func (n *NatGateway) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	natGW, err := n.getNatGateway(ctx, substrate)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting existing NAT GW, %w", err)
	}
	if natGW == nil || *natGW.NatGatewayId == "" {
		return reconcile.Result{}, nil
	}
	if _, err := n.EC2.DeleteNatGatewayWithContext(ctx, &ec2.DeleteNatGatewayInput{
		NatGatewayId: aws.String(*natGW.NatGatewayId),
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("deleting NAT GW, %w", err)
	}
	logging.FromContext(ctx).Infof("Deleted NAT gateway %s", substrate.Name)
	return reconcile.Result{}, nil
}
