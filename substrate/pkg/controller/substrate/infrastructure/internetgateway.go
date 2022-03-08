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
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type InternetGateway struct {
	EC2 *ec2.EC2
}

func (i *InternetGateway) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.Infrastructure.VPCID == nil || substrate.Status.Infrastructure.PublicRouteTableID == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	internetGateway, err := i.ensure(ctx, substrate)
	if err != nil {
		return reconcile.Result{}, err
	}
	if _, err := i.EC2.AttachInternetGatewayWithContext(ctx, &ec2.AttachInternetGatewayInput{InternetGatewayId: internetGateway.InternetGatewayId, VpcId: substrate.Status.Infrastructure.VPCID}); err != nil {
		if err.(awserr.Error).Code() == "Resource.AlreadyAssociated" {
			logging.FromContext(ctx).Debugf("Found internet gateway attachment %s to %s", aws.StringValue(internetGateway.InternetGatewayId), aws.StringValue(substrate.Status.Infrastructure.VPCID))
		} else {
			return reconcile.Result{}, fmt.Errorf("attaching internet gateway, %w", err)
		}
	} else {
		logging.FromContext(ctx).Debugf("Created internet gateway attachment %s to %s", aws.StringValue(internetGateway.InternetGatewayId), aws.StringValue(substrate.Status.Infrastructure.VPCID))
	}
	if _, err := i.EC2.CreateRouteWithContext(ctx, &ec2.CreateRouteInput{
		RouteTableId:         substrate.Status.Infrastructure.PublicRouteTableID,
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            internetGateway.InternetGatewayId,
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("creating route for internet gateway, %w", err)
	} else {
		logging.FromContext(ctx).Debugf("Ensured route for internet gateway %s", aws.StringValue(internetGateway.InternetGatewayId))
	}
	return reconcile.Result{}, nil
}

func (i *InternetGateway) ensure(ctx context.Context, substrate *v1alpha1.Substrate) (*ec2.InternetGateway, error) {
	descrbeInternetGatewaysOutput, err := i.EC2.DescribeInternetGatewaysWithContext(ctx, &ec2.DescribeInternetGatewaysInput{Filters: discovery.Filters(substrate, discovery.Name(substrate))})
	if err != nil {
		return nil, fmt.Errorf("describing internet gateways, %w", err)
	}
	if len(descrbeInternetGatewaysOutput.InternetGateways) > 0 {
		logging.FromContext(ctx).Debugf("Found internet gateway %s", substrate.Name)
		return descrbeInternetGatewaysOutput.InternetGateways[0], nil
	}
	createInternetGatewayOutput, err := i.EC2.CreateInternetGatewayWithContext(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: discovery.Tags(substrate, ec2.ResourceTypeInternetGateway, discovery.Name(substrate)),
	})
	if err != nil {
		return nil, fmt.Errorf("creating internet gateway, %w", err)
	}
	logging.FromContext(ctx).Infof("Created internet gateway %s", substrate.Name)
	return createInternetGatewayOutput.InternetGateway, nil
}

func (i *InternetGateway) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeVpcsOutput, err := i.EC2.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing vpc, %w", err)
	}
	if len(describeVpcsOutput.Vpcs) == 0 {
		return reconcile.Result{}, nil
	}
	describeInternetGatewaysOutput, err := i.EC2.DescribeInternetGatewaysWithContext(ctx, &ec2.DescribeInternetGatewaysInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing internet gateways, %w", err)
	}
	if len(describeInternetGatewaysOutput.InternetGateways) == 0 {
		return reconcile.Result{}, nil
	}
	if _, err := i.EC2.DetachInternetGatewayWithContext(ctx, &ec2.DetachInternetGatewayInput{
		VpcId: describeVpcsOutput.Vpcs[0].VpcId, InternetGatewayId: describeInternetGatewaysOutput.InternetGateways[0].InternetGatewayId,
	}); err != nil {
		if err.(awserr.Error).Code() == "DependencyViolation" {
			return reconcile.Result{Requeue: true}, nil
		}
		if err.(awserr.Error).Code() != "Gateway.NotAttached" {
			return reconcile.Result{}, fmt.Errorf("detaching internet gateway, %w", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted internet gateway %s attachment to %s", aws.StringValue(describeVpcsOutput.Vpcs[0].VpcId), aws.StringValue(describeInternetGatewaysOutput.InternetGateways[0].InternetGatewayId))
	}
	if _, err := i.EC2.DeleteInternetGatewayWithContext(ctx, &ec2.DeleteInternetGatewayInput{
		InternetGatewayId: describeInternetGatewaysOutput.InternetGateways[0].InternetGatewayId,
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("deleting internet gateway, %w", err)
	}
	logging.FromContext(ctx).Infof("Deleted internet gateway %s", aws.StringValue(describeInternetGatewaysOutput.InternetGateways[0].InternetGatewayId))
	return reconcile.Result{}, nil
}
