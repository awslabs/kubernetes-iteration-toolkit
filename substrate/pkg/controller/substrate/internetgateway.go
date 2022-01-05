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
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

type internetGateway struct {
	ec2api *EC2
}

// NewInternetGWController returns a controller for managing internet-gateway in AWS
func NewInternetGWController(ec2api *EC2) *internetGateway {
	return &internetGateway{ec2api: ec2api}
}

// Name returns the name of the controller
func (i *internetGateway) resourceName() string {
	return "internet-gateway"
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the substrate.Status
func (i *internetGateway) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	if substrate.Status.VPCID == nil {
		return fmt.Errorf("vpc ID not found for %v", substrate.Name)
	}
	// Check if the internet gateway exists
	igw, err := i.getInternetGateway(ctx, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting internet-gateway, %w", err)
	}
	// Create an internet-gateway if required
	if igw == nil || aws.StringValue(igw.InternetGatewayId) == "" {
		if igw, err = i.createInternetGateway(ctx, substrate.Name); err != nil {
			return fmt.Errorf("creating internet-gateway, %w", err)
		}
	} else {
		zap.S().Debugf("Successfully discovered internet-gateway %v for cluster %v", *igw.InternetGatewayId, substrate.Name)
	}
	substrate.Status.InternetGatewayID = igw.InternetGatewayId
	// Check igw is attached to the desired VPC ID
	if len(igw.Attachments) == 0 || *igw.Attachments[0].VpcId != *substrate.Status.VPCID {
		if err := i.attachInternetGWToVPC(ctx, *igw.InternetGatewayId, *substrate.Status.VPCID); err != nil {
			return fmt.Errorf("attaching internet-gateway, %w", err)
		}
	}
	return nil
}

// Finalize deletes the resource from AWS
func (i *internetGateway) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	vpc, err := getVPC(ctx, i.ec2api, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting vpc %w", err)
	}
	// Get the internet gateway ID for the control plane
	igw, err := i.getInternetGateway(ctx, substrate.Name)
	if err != nil {
		return err
	}
	if igw != nil && aws.StringValue(igw.InternetGatewayId) != "" {
		// Detach Internet Gateway from VPC
		if _, err := i.ec2api.DetachInternetGatewayWithContext(
			ctx, &ec2.DetachInternetGatewayInput{
				InternetGatewayId: igw.InternetGatewayId,
				VpcId:             aws.String(*vpc.VpcId),
			}); err != nil {
			return fmt.Errorf("detaching internet-gateway from VPC, %w", err)
		}
		// Delete Internet Gateway
		if _, err := i.ec2api.DeleteInternetGatewayWithContext(ctx, &ec2.DeleteInternetGatewayInput{
			InternetGatewayId: igw.InternetGatewayId,
		}); err != nil {
			return fmt.Errorf("deleting internet-gateway, %w", err)
		}
		zap.S().Infof("Successfully deleted internet-gateway %v for cluster %v", *igw.InternetGatewayId, substrate.Name)
	}
	return nil
}

func (i *internetGateway) createInternetGateway(ctx context.Context, identifier string) (*ec2.InternetGateway, error) {
	output, err := i.ec2api.CreateInternetGatewayWithContext(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: generateEC2Tags(i.resourceName(), identifier),
	})
	if err != nil {
		return nil, err
	}
	zap.S().Infof("Successfully created internet-gateway %v for cluster %v", *output.InternetGateway.InternetGatewayId, identifier)
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

func (i *internetGateway) getInternetGateway(ctx context.Context, identifier string) (*ec2.InternetGateway, error) {
	return getInternetGateway(ctx, i.ec2api, identifier)
}

func getInternetGateway(ctx context.Context, ec2api *EC2, identifier string) (*ec2.InternetGateway, error) {
	output, err := ec2api.DescribeInternetGatewaysWithContext(ctx, &ec2.DescribeInternetGatewaysInput{
		Filters: ec2FilterFor(identifier),
	})
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
