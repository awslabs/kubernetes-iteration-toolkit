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

type VPC struct {
	EC2 *ec2.EC2
}

func (v *VPC) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeVpcsOutput, err := v.EC2.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{Filters: discovery.Filters(substrate, discovery.Name(substrate))})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing vpc, %w", err)
	}
	if len(describeVpcsOutput.Vpcs) > 0 {
		substrate.Status.Infrastructure.VPCID = describeVpcsOutput.Vpcs[0].VpcId
		logging.FromContext(ctx).Infof("Found vpc %s", aws.StringValue(substrate.Status.Infrastructure.VPCID))
		return reconcile.Result{}, nil
	}
	createVpcOutput, err := v.EC2.CreateVpc(&ec2.CreateVpcInput{
		// create VPC with a CIDR here and add additional CIDR blocks below
		CidrBlock: aws.String(substrate.Spec.VPC.CIDR[0]),
		TagSpecifications: []*ec2.TagSpecification{{
			ResourceType: aws.String(ec2.ResourceTypeVpc),
			Tags:         discovery.Tags(substrate, discovery.Name(substrate)),
		}},
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("creating VPC, %w", err)
	}
	// add additional CIDRs to VPC here.
	for _, cidr := range substrate.Spec.VPC.CIDR[1:] {
		_, err = v.EC2.AssociateVpcCidrBlock(&ec2.AssociateVpcCidrBlockInput{
			CidrBlock: aws.String(cidr),
			VpcId:     createVpcOutput.Vpc.VpcId,
		})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("associating CIDR to VPC, %w", err)
		}
	}
	substrate.Status.Infrastructure.VPCID = createVpcOutput.Vpc.VpcId
	logging.FromContext(ctx).Infof("Created vpc %s", aws.StringValue(substrate.Status.Infrastructure.VPCID))
	return reconcile.Result{}, err
}

func (v *VPC) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeVpcsOutput, err := v.EC2.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing vpc, %w", err)
	}
	for _, vpc := range describeVpcsOutput.Vpcs {
		if _, err := v.EC2.DeleteVpcWithContext(ctx, &ec2.DeleteVpcInput{VpcId: vpc.VpcId}); err != nil {
			if err.(awserr.Error).Code() == "DependencyViolation" {
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, fmt.Errorf("deleting vpc, %w", err)
		}
		logging.FromContext(ctx).Infof("Deleted vpc %s", aws.StringValue(vpc.VpcId))
	}
	return reconcile.Result{}, nil
}
