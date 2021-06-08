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
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LoadBalancer struct {
	ec2api *awsprovider.EC2
	elbv2  *awsprovider.ELBV2
}

// NewLoadBalancerController returns a controller for managing LoadBalancer in AWS
func NewLoadBalancerController(ec2api *awsprovider.EC2, elbv2 *awsprovider.ELBV2) *LoadBalancer {
	return &LoadBalancer{ec2api: ec2api, elbv2: elbv2}
}

// Name returns the name of the controller
func (n *LoadBalancer) Name() string {
	return "loadbalancer"
}

// For returns the resource this controller is for.
func (n *LoadBalancer) For() controllers.Object {
	return &v1alpha1.LoadBalancer{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (n *LoadBalancer) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	lbObj := object.(*v1alpha1.LoadBalancer)
	output, err := n.getLoadBalancer(ctx, lbObj.Name)
	if err != nil && errors.IsELBLoadBalancerNotExists(err) {
		// If doesn't match or doesn't exists
		output, err = n.createLoadBalancer(ctx, lbObj)
		if err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully created load balancer %v for cluster %v", lbObj.Name, lbObj.Spec.ClusterName)
	} else if err != nil {
		return nil, fmt.Errorf("getting load balancer, %w", err)
	} else {
		zap.S().Debugf("Successfully discovered load balancer %v for cluster %v", lbObj.Name, lbObj.Spec.ClusterName)
	}
	// Check for listeners in ELB
	listenerOutput, err := n.elbv2.DescribeListenersWithContext(ctx, &elbv2.DescribeListenersInput{
		LoadBalancerArn: output.LoadBalancerArn,
	})
	if err != nil {
		return nil, fmt.Errorf("getting listeners, %w", err)
	}
	if listenerOutput == nil || len(listenerOutput.Listeners) == 0 {
		if err := n.createListener(ctx, *output.LoadBalancerArn, lbObj); err != nil {
			return nil, fmt.Errorf("creating listener for %v, %w", lbObj.Name, err)
		}
		zap.S().Infof("Successfully created listener %v for load balancer %v", lbObj.Name, lbObj.Name)
	} else {
		zap.S().Debugf("Successfully discovered listener %v for load balancer %v", lbObj.Name, lbObj.Name)
	}
	return status.Created, nil

}

// Finalize deletes the resource from AWS
func (n *LoadBalancer) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	lbObj := object.(*v1alpha1.LoadBalancer)
	existingLB, err := n.getLoadBalancer(ctx, lbObj.Name)
	if err != nil && errors.IsELBLoadBalancerNotExists(err) {
		return status.Terminated, nil
	} else if err != nil {
		return nil, fmt.Errorf("getting load balancer, %w", err)
	}
	if existingLB != nil {
		if _, err := n.elbv2.DeleteLoadBalancerWithContext(ctx, &elbv2.DeleteLoadBalancerInput{
			LoadBalancerArn: existingLB.LoadBalancerArn,
		}); err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully deleted load balancer %v", *existingLB.LoadBalancerName)
	}
	return status.Terminated, nil
}

func (n *LoadBalancer) createListener(ctx context.Context, lbARN string, lb *v1alpha1.LoadBalancer) error {
	targetGroup, err := getTargetGroup(ctx, n.elbv2, lb.Name)
	if err != nil && errors.IsTargetGroupNotExists(err) {
		return fmt.Errorf("waiting for target group, %w", errors.WaitingForSubResources)
	}
	input := &elbv2.CreateListenerInput{
		DefaultActions: []*elbv2.Action{
			{
				TargetGroupArn: targetGroup.TargetGroupArn,
				Type:           aws.String("forward"),
			},
		},
		LoadBalancerArn: aws.String(lbARN),
		Port:            aws.Int64(int64(lb.Spec.Port)),
		Protocol:        aws.String("TCP"),
	}
	if _, err := n.elbv2.CreateListener(input); err != nil {
		return err
	}
	return err
}

func (n *LoadBalancer) createLoadBalancer(ctx context.Context, lb *v1alpha1.LoadBalancer) (*elbv2.LoadBalancer, error) {
	publicSubnets, err := getPublicSubnetIDs(ctx, n.ec2api, lb.Spec.ClusterName)
	if err != nil {
		return nil, err
	}
	if len(publicSubnets) == 0 {
		return nil, fmt.Errorf("waiting for private subnets, %w", errors.WaitingForSubResources)
	}
	output, err := n.elbv2.CreateLoadBalancerWithContext(ctx, &elbv2.CreateLoadBalancerInput{
		Name:    aws.String(lb.Name),
		Subnets: aws.StringSlice(publicSubnets),
		Tags:    generateLBTags(n.Name(), lb.Spec.ClusterName),
		Type:    aws.String(lb.Spec.Type),
		Scheme:  aws.String(lb.Spec.Scheme),
	})
	if err != nil {
		return nil, fmt.Errorf("creating load balancer %w", err)
	}
	return output.LoadBalancers[0], nil
}

func (n *LoadBalancer) getLoadBalancer(ctx context.Context, name string) (*elbv2.LoadBalancer, error) {
	return getLoadBalancer(ctx, name, n.elbv2)
}

func getLoadBalancer(ctx context.Context, name string, elbv2api *awsprovider.ELBV2) (*elbv2.LoadBalancer, error) {
	output, err := elbv2api.DescribeLoadBalancersWithContext(ctx, &elbv2.DescribeLoadBalancersInput{
		Names: []*string{
			aws.String(name),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe load balancer %w", err)
	}
	if len(output.LoadBalancers) > 1 {
		return nil, fmt.Errorf("expected load balancer count one found count %d", len(output.LoadBalancers))
	}
	return output.LoadBalancers[0], nil
}

func getEtcdLoadBalancer(ctx context.Context, clusterName string, elbv2api *awsprovider.ELBV2) (*elbv2.LoadBalancer, error) {
	return getLoadBalancer(ctx, fmt.Sprintf("%s-%s", clusterName, v1alpha1.ETCDInstances), elbv2api)
}

func getMasterLoadBalancer(ctx context.Context, clusterName string, elbv2api *awsprovider.ELBV2) (*elbv2.LoadBalancer, error) {
	return getLoadBalancer(ctx, fmt.Sprintf("%s-%s", clusterName, v1alpha1.MasterInstances), elbv2api)
}
