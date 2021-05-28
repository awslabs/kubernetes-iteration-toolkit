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
	"github.com/prateekgogia/kit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type vpc struct {
	ec2api *awsprovider.EC2
}

// NewVPCService returns a controller for managing VPCs in AWS
func NewVPCController(ec2api *awsprovider.EC2) *vpc {
	return &vpc{ec2api: ec2api}
}

// Name returns the name of the controller
func (v *vpc) Name() string {
	return "vpc"
}

// For returns the resource this controller is for.
func (v *vpc) For() controllers.Object {
	return &v1alpha1.VPC{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the VPC.Status
// object
func (v *vpc) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	vpcObj := object.(*v1alpha1.VPC)
	// 1. Get the VPC from AWS
	vpc, err := v.getVPC(ctx, vpcObj.Name)
	if err != nil {
		return nil, err
	}
	vpcID := ""
	// 2. If VPC doesn't exist, create a new VPC for this cluster
	if vpc == nil || *vpc.VpcId == "" {
		result, err := v.ec2api.CreateVpc(&ec2.CreateVpcInput{
			CidrBlock:         aws.String(vpcCIDR), // TODO remove hardcoded value
			TagSpecifications: generateEC2Tags(v.Name(), vpcObj.Name),
		})
		if err != nil {
			return nil, err
		}
		vpcID = *result.Vpc.VpcId
		zap.S().Infof("Successfully created VPC ID %v for cluster name %v", vpcID, vpcObj.Name)
	} else {
		vpcID = *vpc.VpcId
		zap.S().Debugf("Successfully discovered VPC ID %v for cluster %v", *vpc.VpcId, vpcObj.Name)
	}
	// 3. Sync resource status with the VPC status object in Kubernetes
	vpcObj.Status.ID = vpcID
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (v *vpc) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	vpcObj := object.(*v1alpha1.VPC)
	if err := v.deleteVPC(ctx, vpcObj.Name); err != nil {
		return nil, err
	}
	zap.S().Infof("Successfully deleted VPC for cluster %v", vpcObj.Name)
	vpcObj.Status.ID = ""
	return status.Terminated, nil
}

func (v *vpc) getVPC(ctx context.Context, clusterName string) (*ec2.Vpc, error) {
	return getVPC(ctx, v.ec2api, clusterName)
}

func getVPC(ctx context.Context, ec2api *awsprovider.EC2, clusterName string) (*ec2.Vpc, error) {
	input := &ec2.DescribeVpcsInput{
		Filters: ec2FilterFor(clusterName),
	}
	output, err := ec2api.DescribeVpcsWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("describing VPC for clusterName %v, err: %w", clusterName, err)
	}
	// Check if VPC doesn't exist return no error
	if len(output.Vpcs) == 0 {
		return nil, nil
	}
	if len(output.Vpcs) > 1 {
		return nil, fmt.Errorf("expected to find one VPC, but found %v", len(output.Vpcs))
	}
	return output.Vpcs[0], nil
}

func (v *vpc) deleteVPC(ctx context.Context, clusterName string) error {
	vpc, err := v.getVPC(ctx, clusterName)
	if err != nil {
		return err
	}
	// vpc doesn't exist, return
	if vpc == nil || *vpc.VpcId == "" {
		return nil
	}
	if _, err := v.ec2api.DeleteVpcWithContext(ctx, &ec2.DeleteVpcInput{
		VpcId: vpc.VpcId,
	}); err != nil {
		return fmt.Errorf("deleting vpc, %w", err)
	}
	return nil
}
