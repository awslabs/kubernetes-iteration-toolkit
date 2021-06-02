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
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type elasticIP struct {
	ec2api *awsprovider.EC2
}

// NewElasticIPController returns a controller for managing elasticIPs in AWS
func NewElasticIPController(ec2api *awsprovider.EC2) *elasticIP {
	return &elasticIP{ec2api: ec2api}
}

// Name returns the name of the controller
func (e *elasticIP) Name() string {
	return "elastic-ip"
}

// For returns the resource this controller is for.
func (e *elasticIP) For() controllers.Object {
	return &v1alpha1.ElasticIP{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the eipObj.Status
// object
func (e *elasticIP) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	eipObj := object.(*v1alpha1.ElasticIP)
	// 1. Check if the elastic IP for this cluster already exists in AWS
	eip, err := e.getElasticIP(ctx, eipObj.Name)
	if err != nil {
		return nil, fmt.Errorf("getting elastic-ip, %w", err)
	}
	publicIP := ""
	// 2. create an elastic-ip in AWS if required
	if eip == nil {
		output, err := e.createElasticIP(ctx, eipObj.Name)
		if err != nil {
			return nil, fmt.Errorf("creating elastic-ip, %w", err)
		}
		publicIP = *output.PublicIp
	} else {
		publicIP = *eip.PublicIp
		zap.S().Debugf("Successfully discovered elastic-ip %v for cluster %v", *eip.AllocationId, eipObj.Name)
	}
	eipObj.Status.PublicIP = publicIP
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (e *elasticIP) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	eipObj := object.(*v1alpha1.ElasticIP)
	if err := e.deleteElasticIP(ctx, eipObj.Name); err != nil {
		return nil, err
	}
	eipObj.Status.PublicIP = ""
	return status.Terminated, nil
}

func (e *elasticIP) createElasticIP(ctx context.Context, clusterName string) (*ec2.AllocateAddressOutput, error) {
	output, err := e.ec2api.AllocateAddressWithContext(ctx, &ec2.AllocateAddressInput{
		TagSpecifications: generateEC2Tags(e.Name(), clusterName),
	})
	if err != nil {
		return nil, err
	}
	zap.S().Infof("Successfully created elastic-ip %v for cluster %v", *output.AllocationId, clusterName)
	return output, nil
}

func (e *elasticIP) deleteElasticIP(ctx context.Context, clusterName string) error {
	eip, err := e.getElasticIP(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("getting elastic IP, %w", err)
	}
	if eip == nil || aws.StringValue(eip.AllocationId) == "" {
		return nil
	}
	if _, err := e.ec2api.ReleaseAddressWithContext(ctx, &ec2.ReleaseAddressInput{
		AllocationId: eip.AllocationId,
	}); err != nil {
		return fmt.Errorf("failed to release elastic IP, %w", err)
	}
	zap.S().Infof("Successfully deleted elastic-ip %v for cluster %v", *eip.AllocationId, clusterName)
	return nil
}

func (e *elasticIP) getElasticIP(ctx context.Context, clusterName string) (*ec2.Address, error) {
	return getElasticIP(ctx, e.ec2api, clusterName)
}

func getElasticIP(ctx context.Context, ec2api *awsprovider.EC2, clusterName string) (*ec2.Address, error) {
	output, err := ec2api.DescribeAddressesWithContext(ctx, &ec2.DescribeAddressesInput{
		Filters: ec2FilterFor(clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("describing elastic-ip, %w", err)
	}
	if len(output.Addresses) == 0 {
		return nil, nil
	}
	if len(output.Addresses) > 1 {
		return nil, fmt.Errorf("expected to find one elastic-ip, but found %d", len(output.Addresses))
	}
	return output.Addresses[0], nil
}
