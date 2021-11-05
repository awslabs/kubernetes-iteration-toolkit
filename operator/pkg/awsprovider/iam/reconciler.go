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

package iam

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	apis "github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/awsprovider"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"go.uber.org/zap"

	"knative.dev/pkg/ptr"
)

type Controller struct {
	iam        *awsprovider.IAM
	kubeClient *kubeprovider.Client
}

// NewController returns a controller for managing IAM resources in AWS
func NewController(iam *awsprovider.IAM, client *kubeprovider.Client) *Controller {
	return &Controller{iam: iam, kubeClient: client}
}

var (
	kitNodeRolePolicies = []string{
		"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
		"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
		"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	}
)

func (c *Controller) Reconcile(ctx context.Context, controlPlane *apis.ControlPlane) error {
	role, err := c.getRole(ctx, KitNodeRoleNameFor(controlPlane.ClusterName()))
	if err != nil && !errors.IsIAMObjectDoNotExist(err) {
		return fmt.Errorf("getting IAM role for %v, %w", controlPlane.ClusterName(), err)
	}
	if role == nil {
		role, err = c.createRole(ctx, &iam.CreateRoleInput{
			AssumeRolePolicyDocument: aws.String(assumeRolePolicyDocument),
			Description:              aws.String("Role assumed by dataplane nodes created by KIT operated"),
			RoleName:                 aws.String(KitNodeRoleNameFor(controlPlane.ClusterName())),
			Tags:                     generateRoleTags(controlPlane.ClusterName()),
		})
		if err != nil {
			return fmt.Errorf("creating IAM role for %v, %w", controlPlane.ClusterName(), err)
		}
		zap.S().Infof("[%s] Created IAM Role %v", controlPlane.ClusterName(), aws.StringValue(role.RoleName))
	}
	if err := role.addRoleToInstanceProfile(ctx, KitNodeRoleNameFor(controlPlane.ClusterName()),
		KitNodeInstanceProfileNameFor(controlPlane.ClusterName())); err != nil {
		return fmt.Errorf("adding instance profile to role, %w", err)
	}
	for _, policy := range kitNodeRolePolicies {
		if err := role.attachPolicy(ctx, policy, KitNodeRoleNameFor(controlPlane.ClusterName())); err != nil {
			return fmt.Errorf("attaching policies to role, %w", err)
		}
	}
	return nil
}

func (c *Controller) Finalize(ctx context.Context, controlPlane *apis.ControlPlane) error {
	_, err := c.iam.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{
		InstanceProfileName: aws.String(KitNodeInstanceProfileNameFor(controlPlane.ClusterName())),
		RoleName:            aws.String(KitNodeRoleNameFor(controlPlane.ClusterName())),
	})
	if err != nil && !errors.IsIAMObjectDoNotExist(err) {
		return fmt.Errorf("removing role from instance profile, %w", err)
	}
	_, err = c.iam.DeleteInstanceProfileWithContext(ctx, &iam.DeleteInstanceProfileInput{
		InstanceProfileName: aws.String(KitNodeInstanceProfileNameFor(controlPlane.ClusterName())),
	})
	if err != nil && !errors.IsIAMObjectDoNotExist(err) {
		return fmt.Errorf("deleting instance profile, %w", err)
	}

	for _, policy := range kitNodeRolePolicies {
		if _, err = c.iam.DetachRolePolicyWithContext(ctx, &iam.DetachRolePolicyInput{
			PolicyArn: aws.String(policy),
			RoleName:  aws.String(KitNodeRoleNameFor(controlPlane.ClusterName())),
		}); err != nil {
			return fmt.Errorf("detaching policy from role, %w", err)
		}
	}
	_, err = c.iam.DeleteRoleWithContext(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(KitNodeRoleNameFor(controlPlane.ClusterName())),
	})
	if err != nil && !errors.IsIAMObjectDoNotExist(err) {
		return fmt.Errorf("deleting role, %w", err)
	}
	zap.S().Infof("[%s] Deleted IAM Role %v and instance profile",
		controlPlane.ClusterName(), KitNodeRoleNameFor(controlPlane.ClusterName()))
	return nil
}

func (c *Controller) getRole(ctx context.Context, roleName string) (*role, error) {
	roleOutput, err := c.iam.GetRoleWithContext(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, fmt.Errorf("getting iam role %v, %w", roleName, err)
	}
	return &role{iam: c.iam, Role: roleOutput.Role}, nil
}

func (c *Controller) createRole(ctx context.Context, roleInput *iam.CreateRoleInput) (*role, error) {
	roleOutput, err := c.iam.CreateRoleWithContext(ctx, roleInput)
	return &role{iam: c.iam, Role: roleOutput.Role}, err
}

type role struct {
	iam *awsprovider.IAM
	*iam.Role
}

func (r *role) addRoleToInstanceProfile(ctx context.Context, roleName, profileName string) error {
	profile, err := createInstanceProfile(ctx, r.iam, profileName)
	if err != nil {
		return fmt.Errorf("creating instance profile, %w", err)
	}
	if profile == nil {
		return fmt.Errorf("instance profile is NIL")
	}
	for _, role := range profile.Roles {
		if aws.StringValue(role.RoleName) == roleName {
			return nil
		}
	}
	_, err = r.iam.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
		RoleName:            aws.String(roleName),
	})
	return err
}

func (r *role) attachPolicy(ctx context.Context, policyARN, roleName string) error {
	_, err := r.iam.AttachRolePolicy(&iam.AttachRolePolicyInput{
		PolicyArn: aws.String(policyARN),
		RoleName:  aws.String(roleName),
	})
	if errors.IsIAMObjectAlreadyExist(err) {
		return nil
	}
	return err
}

func createInstanceProfile(ctx context.Context, iamAPI *awsprovider.IAM, profileName string) (*iam.InstanceProfile, error) {
	profile, err := iamAPI.CreateInstanceProfileWithContext(ctx, &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	if errors.IsIAMObjectAlreadyExist(err) {
		output, err := iamAPI.GetInstanceProfileWithContext(ctx, &iam.GetInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
		})
		if err != nil {
			return nil, err
		}
		return output.InstanceProfile, nil
	}
	if err != nil {
		return nil, fmt.Errorf("creating instance profile, %w", err)
	}
	return profile.InstanceProfile, nil
}

func KitNodeRoleNameFor(clusterName string) string {
	return fmt.Sprintf("KitDataplaneNodes-%s-cluster", clusterName)
}

func KitNodeInstanceProfileNameFor(clusterName string) string {
	return fmt.Sprintf("KitDataplaneNodesProfile-%s-cluster", clusterName)
}

func generateRoleTags(clusterName string) []*iam.Tag {
	return []*iam.Tag{{
		Key:   ptr.String(fmt.Sprintf("kubernetes.io/cluster/%s", clusterName)),
		Value: ptr.String("owned"),
	}}
}

// KitNodeRole is assumed by the nodes provisioned by kit-operator for dataplane
const assumeRolePolicyDocument = `{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Action": "sts:AssumeRole",
			"Effect": "Allow",
			"Principal": {
				"Service": "ec2.amazonaws.com"
			}
		}
	]
}`
