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
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"github.com/prateekgogia/kit/pkg/errors"
	"github.com/prateekgogia/kit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type iamPolicy struct {
	iam *awsprovider.IAM
}

// NewIAMPolicyController returns a controller for managing IAM policy in AWS
func NewIAMPolicyController(iam *awsprovider.IAM) *iamPolicy {
	return &iamPolicy{iam: iam}
}

// Name returns the name of the controller
func (i *iamPolicy) Name() string {
	return "iam-policy"
}

// For returns the resource this controller is for.
func (i *iamPolicy) For() controllers.Object {
	return &v1alpha1.ControlPlane{}
}

const (
	MasterInstancePolicyName = "master-instance-policy"
	ETCDInstancePolicyName   = "etcd-instance-policy"
)

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (i *iamPolicy) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	roles := []string{
		fmt.Sprintf(MasterInstanceRoleName, controlPlane.Name),
		fmt.Sprintf(ETCDInstanceRoleName, controlPlane.Name),
	}
	for _, roleName := range roles {
		desiredPolicies := i.policyRoleMapping(roleName, controlPlane.Name)
		for policyName, desiredPolicy := range desiredPolicies {
			// check role exists
			if _, err := getRole(ctx, i.iam, roleName); err != nil && errors.IsIAMResourceNotFound(err) {
				return nil, nil
			} else if err != nil {
				return nil, fmt.Errorf("getting role, %w", err)
			}
			// check policy exists on the role
			output, err := i.getRolePolicy(ctx, policyName, roleName)
			if err != nil && !errors.IsIAMResourceNotFound(err) {
				return nil, fmt.Errorf("getting role policy, %w", err)
			}
			if !policyFoundEqualsDesired(output, desiredPolicy) {
				// Policy is not found or doesn't match the desired policy
				if _, err := i.iam.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{
					RoleName:       aws.String(roleName),
					PolicyName:     aws.String(policyName),
					PolicyDocument: aws.String(desiredPolicy),
				}); err != nil {
					return nil, fmt.Errorf("adding policy to role, %w", err)
				}
				zap.S().Infof("Successfully added policy %v to role %v", policyName, roleName)
				continue
			}
			zap.S().Debugf("Successfully discovered policy %v to role %v", policyName, roleName)
		}
	}
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (i *iamPolicy) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	roles := []string{
		fmt.Sprintf(MasterInstanceRoleName, controlPlane.Name),
		fmt.Sprintf(ETCDInstanceRoleName, controlPlane.Name),
	}
	for _, roleName := range roles {
		desiredPolicies := i.policyRoleMapping(roleName, controlPlane.Name)
		for policyName := range desiredPolicies {
			_, err := i.iam.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{
				PolicyName: aws.String(policyName),
				RoleName:   aws.String(roleName),
			})
			if err != nil {
				zap.S().Errorf("Failed to delete role policy, %v", err)
				return nil, err
			}
			zap.S().Infof("Successfully removed policy %s from role %s", policyName, roleName)
		}
	}
	return status.Terminated, nil
}

func (i *iamPolicy) policyRoleMapping(roleName, clusterName string) map[string]string {
	switch roleName {
	case fmt.Sprintf(MasterInstanceRoleName, clusterName):
		return map[string]string{MasterInstancePolicyName: masterPolicy}
	case fmt.Sprintf(ETCDInstanceRoleName, clusterName):
		return map[string]string{ETCDInstancePolicyName: etcdPolicy}
	}
	return map[string]string{}
}

func (i *iamPolicy) getRolePolicy(ctx context.Context, policy, role string) (*iam.GetRolePolicyOutput, error) {
	output, err := i.iam.GetRolePolicyWithContext(ctx, &iam.GetRolePolicyInput{
		PolicyName: aws.String(policy),
		RoleName:   aws.String(role),
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

func policyFoundEqualsDesired(output *iam.GetRolePolicyOutput, expectedPolicy string) bool {
	if output != nil {
		decodedPolicyDoc, err := url.QueryUnescape(*output.PolicyDocument)
		if err != nil {
			zap.S().Errorf("Failed to decode policy document, %v", err)
			return false
		}
		return decodedPolicyDoc == expectedPolicy
	}
	return false
}

var (
	masterPolicy = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Action": [
                "ec2:DescribeInstances",
                "ec2:DescribeTags",
                "ec2:AssociateTrunkInterface",
                "ecr:GetAuthorizationToken"
            ],
            "Resource": "*",
            "Effect": "Allow"
        },
		{
            "Action": [
                "ec2:DescribeInstances",
                "ec2:DescribeTags",
                "ec2:AssociateTrunkInterface",
                "ecr:GetAuthorizationToken"
            ],
            "Resource": "*",
            "Effect": "Allow"
        }
    ]
}`

	etcdPolicy = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Action": [
                "ecr:GetAuthorizationToken"
            ],
            "Resource": "*",
            "Effect": "Allow"
        }
    ]
}`
)
