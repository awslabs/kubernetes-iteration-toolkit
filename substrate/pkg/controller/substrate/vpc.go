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
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type vpc struct {
	ec2Client *ec2.EC2
}

func (v *vpc) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeVpcsOutput, err := v.ec2Client.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{Filters: filtersFor(substrate.Name)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing vpc, %w", err)
	}
	if len(describeVpcsOutput.Vpcs) > 0 {
		substrate.Status.VPCID = describeVpcsOutput.Vpcs[0].VpcId
		logging.FromContext(ctx).Infof("Found vpc %s", aws.StringValue(substrate.Status.VPCID))
		return reconcile.Result{}, nil
	}
	createVpcOutput, err := v.ec2Client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock:         aws.String(substrate.Spec.VPC.CIDR),
		TagSpecifications: tagsFor(ec2.ResourceTypeVpc, substrate.Name, substrate.Name),
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("creating VPC, %w", err)
	}
	substrate.Status.VPCID = createVpcOutput.Vpc.VpcId
	logging.FromContext(ctx).Infof("Created vpc %s", aws.StringValue(substrate.Status.VPCID))
	return reconcile.Result{}, err
}

func (v *vpc) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeVpcsOutput, err := v.ec2Client.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{Filters: filtersFor(substrate.Name)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing vpc, %w", err)
	}
	if len(describeVpcsOutput.Vpcs) == 0 {
		return reconcile.Result{}, nil
	}
	if _, err := v.ec2Client.DeleteVpcWithContext(ctx, &ec2.DeleteVpcInput{VpcId: describeVpcsOutput.Vpcs[0].VpcId}); err != nil {
		if err.(awserr.Error).Code() == "DependencyViolation" {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, fmt.Errorf("deleting vpc, %w", err)
	}
	logging.FromContext(ctx).Infof("Deleted vpc %s", aws.StringValue(describeVpcsOutput.Vpcs[0].VpcId))
	return reconcile.Result{}, nil
}
