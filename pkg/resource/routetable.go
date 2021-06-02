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

	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RouteTable struct {
	KubeClient client.Client
}

const (
	publicSubnets  = "public-subnets"
	privateSubnets = "private-subnets"
)

func (r *RouteTable) Create(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	for _, subnetType := range []string{publicSubnets, privateSubnets} {
		if err := r.exists(ctx, controlPlane.Namespace, ObjectName(controlPlane, subnetType)); err != nil {
			if errors.IsNotFound(err) {
				if err := r.create(ctx, subnetType, controlPlane); err != nil {
					return fmt.Errorf("creating route table kube object, %w", err)
				}
				continue
			}
			return fmt.Errorf("getting route table object, %w", err)
		}
	}
	// TODO verify existing object matches the desired else update
	return nil
}

func (r *RouteTable) create(ctx context.Context, subnetType string, controlPlane *v1alpha1.ControlPlane) error {
	forPrivateSubnets := true
	if subnetType == publicSubnets {
		forPrivateSubnets = false
	}
	if err := r.KubeClient.Create(ctx, &v1alpha1.RouteTable{
		ObjectMeta: ObjectMeta(controlPlane, subnetType),
		Spec: v1alpha1.RouteTableSpec{
			ForPrivateSubnets: forPrivateSubnets,
			ClusterName:       controlPlane.Name,
		},
	}); err != nil {
		return fmt.Errorf("creating route table kube object, %w", err)
	}
	zap.S().Debugf("Successfully created route table object for cluster %v", controlPlane.Name)
	return nil
}

func (r *RouteTable) exists(ctx context.Context, ns, objName string) error {
	result := &v1alpha1.RouteTable{}
	if err := r.KubeClient.Get(ctx, NamespacedName(ns, objName), result); err != nil {
		return err
	}
	return nil
}
