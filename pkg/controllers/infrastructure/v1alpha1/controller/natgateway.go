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
	"github.com/prateekgogia/kit/pkg/errors"
	"github.com/prateekgogia/kit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type natGateway struct {
	ec2api *awsprovider.EC2
}

// NewNatGWController returns a controller for managing nat-gateway in AWS
func NewNatGWController(ec2api *awsprovider.EC2) *natGateway {
	return &natGateway{ec2api: ec2api}
}

// Name returns the name of the controller
func (n *natGateway) Name() string {
	return "natgateway"
}

// For returns the resource this controller is for.
func (n *natGateway) For() controllers.Object {
	return &v1alpha1.NatGateway{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the natGwObj.Status
// object
func (n *natGateway) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	natGwObj := object.(*v1alpha1.NatGateway)
	// 1. Check if the Nat gateway exists in AWS
	natGW, err := n.getNatGateway(ctx, natGwObj.Name)
	if err != nil {
		return nil, fmt.Errorf("getting nat-gateway, %w", err)
	}
	// 2. create a nat-gateway in AWS if required
	if natGW == nil || aws.StringValue(natGW.NatGatewayId) == "" {
		if natGW, err = n.createNatGateway(ctx, natGwObj); err != nil {
			return nil, fmt.Errorf("creating nat-gateway, %w", err)
		}
	} else {
		zap.S().Debugf("Successfully discovered nat-gateway %v for cluster %v", *natGW.NatGatewayId, natGwObj.Name)
	}
	natGWID := ""
	if natGW != nil {
		natGWID = aws.StringValue(natGW.NatGatewayId)
	}
	natGwObj.Status.ID = natGWID
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (n *natGateway) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	natGwObj := object.(*v1alpha1.NatGateway)
	if err := n.deleteNatGateway(ctx, natGwObj.Name); err != nil {
		return nil, err
	}
	natGwObj.Status.ID = ""
	return status.Terminated, nil
}

func (n *natGateway) createNatGateway(ctx context.Context, natGwObj *v1alpha1.NatGateway) (*ec2.NatGateway, error) {
	// 1. Get elastic IP ID
	elasticIP, err := getElasticIP(ctx, n.ec2api, natGwObj.Name)
	if err != nil {
		return nil, err
	}
	if elasticIP == nil || aws.StringValue(elasticIP.AllocationId) == "" {
		return nil, fmt.Errorf("elastic IP does not exist %w", errors.WaitingForSubResources)
	}
	// 2. Get a private subnet ID
	privateSubnets, err := getPrivateSubnetIDs(ctx, n.ec2api, natGwObj.Name)
	if err != nil {
		return nil, fmt.Errorf("getting private subnets, %w", err)
	}
	privateSubnetID := privateSubnets[0]
	// 3. Create NAT Gateway
	output, err := n.ec2api.CreateNatGatewayWithContext(ctx, &ec2.CreateNatGatewayInput{
		AllocationId:      elasticIP.AllocationId,
		SubnetId:          aws.String(privateSubnetID),
		TagSpecifications: generateEC2Tags(n.Name(), natGwObj.Name),
	})
	if err != nil {
		return nil, err
	}
	zap.S().Infof("Successfully created nat-gateway %v for cluster %v", *output.NatGateway.NatGatewayId, natGwObj.Name)
	// 3. Wait for the NAT Gateway to be available
	// There are scenarios where after creating a NAT gateway, describe NAT GW
	// call doesn't return the NatGateway ID we just created. In such cases, we
	// end up creating multiple gateways, in the end only one becomes available
	// and others end up in the failed state.
	zap.S().Infof("Waiting for nat-gateway %v to be available for cluster %v", *output.NatGateway.NatGatewayId, natGwObj.Name)
	if err := n.ec2api.WaitUntilNatGatewayAvailableWithContext(ctx, &ec2.DescribeNatGatewaysInput{
		NatGatewayIds: []*string{output.NatGateway.NatGatewayId},
	}); err != nil {
		return nil, err
	}
	return output.NatGateway, nil
}

func (n *natGateway) deleteNatGateway(ctx context.Context, clusterName string) error {
	natGW, err := n.getNatGateway(ctx, clusterName)
	if err != nil {
		return err
	}
	if natGW == nil || *natGW.NatGatewayId == "" {
		return nil
	}
	if _, err := n.ec2api.DeleteNatGatewayWithContext(ctx, &ec2.DeleteNatGatewayInput{
		NatGatewayId: aws.String(*natGW.NatGatewayId),
	}); err != nil {
		return err
	}
	zap.S().Infof("Successfully deleted nat-gateway %v for cluster %v", *natGW.NatGatewayId, clusterName)
	return nil
}

func (n *natGateway) getNatGateway(ctx context.Context, clusterName string) (*ec2.NatGateway, error) {
	return getNatGateway(ctx, n.ec2api, clusterName)
}

func getNatGateway(ctx context.Context, ec2api *awsprovider.EC2, clusterName string) (*ec2.NatGateway, error) {
	output, err := ec2api.DescribeNatGatewaysWithContext(ctx, &ec2.DescribeNatGatewaysInput{
		Filter: ec2FilterFor(clusterName),
	})
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
