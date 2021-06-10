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

package resource

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SecurityGroup struct {
	KubeClient client.Client
}

func (s *SecurityGroup) Create(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	for _, component := range v1alpha1.ComponentsSupported {
		if err := s.exists(ctx, controlPlane.Namespace, ObjectName(controlPlane, component)); err != nil {
			if errors.KubeObjNotFound(err) {
				if err := s.create(ctx, component, controlPlane); err != nil {
					return fmt.Errorf("creating security group kube object, %w", err)
				}
				continue
			}
			return fmt.Errorf("getting security group object, %w", err)
		}
	}
	// TODO verify existing object matches the desired else update
	return nil
}

func (s *SecurityGroup) create(ctx context.Context, component string, controlPlane *v1alpha1.ControlPlane) error {
	groupName := v1alpha1.GroupName(controlPlane.Name, component)
	if err := s.KubeClient.Create(ctx, &v1alpha1.SecurityGroup{
		ObjectMeta: ObjectMeta(controlPlane, component),
		Spec: v1alpha1.SecurityGroupSpec{
			GroupName:   groupName,
			ClusterName: controlPlane.Name,
			Permissions: s.createPermissionsFor(ctx, groupName, controlPlane),
		},
	}); err != nil {
		return fmt.Errorf("creating security group kube object, %w", err)
	}
	zap.S().Debugf("Successfully created security group object for cluster %v", controlPlane.Name)
	return nil
}

func (s *SecurityGroup) exists(ctx context.Context, ns, objName string) error {
	result := &v1alpha1.SecurityGroup{}
	if err := s.KubeClient.Get(ctx, NamespacedName(ns, objName), result); err != nil {
		return err
	}
	return nil
}

func (s *SecurityGroup) createPermissionsFor(ctx context.Context, groupName string, controlPlane *v1alpha1.ControlPlane) []*v1alpha1.IpPermission {
	switch groupName {
	case v1alpha1.GroupName(controlPlane.Name, v1alpha1.MasterInstances):
		return []*v1alpha1.IpPermission{{
			FromPort:   aws.Int64(controlPlane.MasterSecurePortInt64()),
			ToPort:     aws.Int64(controlPlane.MasterSecurePortInt64()),
			IpProtocol: aws.String("tcp"),
			CidrIP:     aws.String("0.0.0.0/0"),
		}}
	case v1alpha1.GroupName(controlPlane.Name, v1alpha1.ETCDInstances):
		// gid, err := s.getMasterSecurityGroupID(ctx, controlPlane.Namespace, groupName)
		// if err != nil && errors.KubeObjNotFound(err) {
		// 	zap.S().Errorf("waiting for master security group ID, %w", errors.WaitingForSubResources)
		// }
		return []*v1alpha1.IpPermission{{
			FromPort:   aws.Int64(2379),
			ToPort:     aws.Int64(2380),
			IpProtocol: aws.String("tcp"),
			GroupName:  aws.String(groupName),
		}, {
			FromPort:   aws.Int64(2379),
			ToPort:     aws.Int64(2379),
			IpProtocol: aws.String("tcp"),
			GroupName:  aws.String(v1alpha1.GroupName(controlPlane.Name, v1alpha1.MasterInstances)),
		}}
	}
	return nil
}
