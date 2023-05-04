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

package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Address struct {
	EC2 *ec2.EC2
}

type addressOutput struct {
	ipAddress    *string
	allocationID *string
}

func (a *Address) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	for _, req := range []struct {
		name *string
	}{
		{name: discovery.Name(substrate, "apiserver")},
		{name: discovery.Name(substrate, "natgw")},
	} {
		result, err := a.ensure(ctx, req.name, substrate)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("ensuring IP address, %w", err)
		}
		switch aws.StringValue(req.name) {
		case aws.StringValue(discovery.Name(substrate, "apiserver")):
			substrate.Status.Cluster.APIServerAddress = result.ipAddress
		case aws.StringValue(discovery.Name(substrate, "natgw")):
			substrate.Status.Infrastructure.ElasticIpIDForNatGW = result.allocationID
		}
	}
	return reconcile.Result{}, nil
}

func (a *Address) ensure(ctx context.Context, tagValue *string, substrate *v1alpha1.Substrate) (*addressOutput, error) {
	addressesOutput, err := a.EC2.DescribeAddressesWithContext(ctx, &ec2.DescribeAddressesInput{Filters: discovery.Filters(substrate, tagValue)})
	if err != nil {
		return nil, fmt.Errorf("describing addresses, %w", err)
	}
	if len(addressesOutput.Addresses) > 0 {
		logging.FromContext(ctx).Debugf("Found address, name %s, IP %s", aws.StringValue(tagValue), aws.StringValue(addressesOutput.Addresses[0].PublicIp))
		return &addressOutput{addressesOutput.Addresses[0].PublicIp, addressesOutput.Addresses[0].AllocationId}, nil
	}
	output, err := a.EC2.AllocateAddressWithContext(ctx, &ec2.AllocateAddressInput{
		TagSpecifications: []*ec2.TagSpecification{{
			ResourceType: aws.String(ec2.ResourceTypeElasticIp),
			Tags:         discovery.Tags(substrate, tagValue),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("allocating address, %w", err)
	}
	logging.FromContext(ctx).Infof("Created address name %s, IP %s", aws.StringValue(tagValue), aws.StringValue(output.PublicIp))
	return &addressOutput{output.PublicIp, output.AllocationId}, nil
}

func (a *Address) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	addressesOutput, err := a.EC2.DescribeAddressesWithContext(ctx, &ec2.DescribeAddressesInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing addresses, %w", err)
	}
	for _, address := range addressesOutput.Addresses {
		if address.AssociationId != nil {
			if _, err := a.EC2.DisassociateAddressWithContext(ctx, &ec2.DisassociateAddressInput{AssociationId: address.AssociationId}); err != nil {
				if strings.Contains(err.Error(), "InvalidAssociationID.NotFound") {
					// Association ID does not exist, continue with the next address
					continue
					// Until NAT GW is not yet deleted we can't disassociate, check for error and retry
				} else if strings.Contains(err.Error(), "AuthFailure: You do not have permission to access the specified resource") {
					return reconcile.Result{Requeue: true}, nil
				} else {
					return reconcile.Result{}, fmt.Errorf("disassociating elastic IP, %w", err)
				}
			}
		}
		if _, err := a.EC2.ReleaseAddressWithContext(ctx, &ec2.ReleaseAddressInput{AllocationId: address.AllocationId}); err != nil {
			return reconcile.Result{}, fmt.Errorf("releasing elastic IP, %w", err)
		}
	}
	return reconcile.Result{}, nil
}
