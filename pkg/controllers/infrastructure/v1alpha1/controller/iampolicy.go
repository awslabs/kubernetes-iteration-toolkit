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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
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
	return &v1alpha1.Policy{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (i *iamPolicy) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	policyObj := object.(*v1alpha1.Policy)
	policyName := v1alpha1.PolicyName(policyObj.Name)
	roleName := fmt.Sprintf("%s-role", policyObj.Name)
	policyDocument := i.getPolicyDocument(policyObj.Name)
	// check role exists
	if _, err := getRole(ctx, i.iam, roleName); err != nil && errors.IsIAMResourceNotFound(err) {
		return nil, fmt.Errorf("waiting for the role to be created, %w", errors.WaitingForSubResources)
	} else if err != nil {
		return nil, fmt.Errorf("getting role, %w", err)
	}
	// check policy exists on the role
	output, err := i.getRolePolicy(ctx, policyName, roleName)
	if err != nil && !errors.IsIAMResourceNotFound(err) {
		return nil, fmt.Errorf("getting role policy, %w", err)
	}
	if !policyFoundEqualsDesired(output, policyDocument) {
		// Policy is not found or doesn't match the desired policy
		if _, err := i.iam.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{
			RoleName:       aws.String(roleName),
			PolicyName:     aws.String(policyName),
			PolicyDocument: aws.String(policyDocument),
		}); err != nil {
			return nil, fmt.Errorf("adding policy to role, %w", err)
		}
		zap.S().Infof("Successfully added policy %v to role %v", policyName, roleName)
	} else {
		zap.S().Debugf("Successfully discovered policy %v to role %v", policyName, roleName)
	}
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (i *iamPolicy) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	policyObj := object.(*v1alpha1.Policy)
	policyName := v1alpha1.PolicyName(policyObj.Name)
	roleName := fmt.Sprintf("%s-role", policyObj.Name)
	if _, err := i.iam.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{
		PolicyName: aws.String(policyName),
		RoleName:   aws.String(roleName),
	}); err != nil {
		zap.S().Errorf("Failed to delete role policy, %v", err)
		return nil, err
	}
	zap.S().Infof("Successfully removed policy %s from role %s", policyName, roleName)
	return status.Terminated, nil
}

func (i *iamPolicy) getPolicyDocument(policyName string) string {
	switch {
	case strings.Contains(policyName, v1alpha1.MasterInstances):
		return masterPolicy
	case strings.Contains(policyName, v1alpha1.ETCDInstances):
		return etcdPolicy
	}
	return ""
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
