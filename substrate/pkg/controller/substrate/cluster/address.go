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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Address struct {
	EC2 *ec2.EC2
}

func (a *Address) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	addressesOutput, err := a.EC2.DescribeAddressesWithContext(ctx, &ec2.DescribeAddressesInput{Filters: discovery.Filters(substrate, discovery.Name(substrate))})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing addresses, %w", err)
	}
	if len(addressesOutput.Addresses) > 0 {
		logging.FromContext(ctx).Infof("Found address %s", aws.StringValue(addressesOutput.Addresses[0].PublicIp))
		substrate.Status.Cluster.Address = addressesOutput.Addresses[0].PublicIp
		return reconcile.Result{}, nil
	}
	addressOutput, err := a.EC2.AllocateAddressWithContext(ctx, &ec2.AllocateAddressInput{TagSpecifications: discovery.Tags(substrate, ec2.ResourceTypeElasticIp, discovery.Name(substrate))})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("allocating address, %w", err)
	}
	logging.FromContext(ctx).Infof("Created address %s", aws.StringValue(addressOutput.PublicIp))
	substrate.Status.Cluster.Address = addressOutput.PublicIp
	return reconcile.Result{}, nil
}

func (a *Address) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	addressesOutput, err := a.EC2.DescribeAddressesWithContext(ctx, &ec2.DescribeAddressesInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing addresses, %w", err)
	}
	for _, address := range addressesOutput.Addresses {
		if address.AssociationId != nil {
			if _, err := a.EC2.DisassociateAddressWithContext(ctx, &ec2.DisassociateAddressInput{AssociationId: address.AssociationId}); err != nil {
				return reconcile.Result{}, fmt.Errorf("disassociating elastic IP, %w", err)
			}
		}
		if _, err := a.EC2.ReleaseAddressWithContext(ctx, &ec2.ReleaseAddressInput{AllocationId: address.AllocationId}); err != nil {
			return reconcile.Result{}, fmt.Errorf("releasing elastic IP, %w", err)
		}
	}
	return reconcile.Result{}, nil
}
