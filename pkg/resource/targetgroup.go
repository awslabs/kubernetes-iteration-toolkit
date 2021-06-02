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

type TargetGroup struct {
	KubeClient client.Client
}

func (t *TargetGroup) Create(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	for _, component := range v1alpha1.ComponentsSupported {
		if err := t.exists(ctx, controlPlane.Namespace, ObjectName(controlPlane, component)); err != nil {
			if errors.IsNotFound(err) {
				if err := t.create(ctx, component, controlPlane); err != nil {
					return fmt.Errorf("creating target group kube object, %w", err)
				}
				continue
			}
			return fmt.Errorf("getting target group object, %w", err)
		}
	}
	// TODO verify existing object matches the desired else update
	return nil
}

func (t *TargetGroup) create(ctx context.Context, component string, controlPlane *v1alpha1.ControlPlane) error {
	input := &v1alpha1.TargetGroup{
		ObjectMeta: ObjectMeta(controlPlane, component),
		Spec: v1alpha1.TargetGroupSpec{
			ClusterName: controlPlane.Name,
		},
	}
	switch component {
	case v1alpha1.MasterInstances:
		input.Spec.Port = 443
	case v1alpha1.ETCDInstances:
		input.Spec.Port = 2379
	}
	if err := t.KubeClient.Create(ctx, input); err != nil {
		return fmt.Errorf("creating target group kube object, %w", err)
	}
	zap.S().Debugf("Successfully created target group object %v for cluster %v",
		ObjectMeta(controlPlane, component).Name, controlPlane.Name)
	return nil
}

func (t *TargetGroup) exists(ctx context.Context, ns, objName string) error {
	result := &v1alpha1.TargetGroup{}
	if err := t.KubeClient.Get(ctx, NamespacedName(ns, objName), result); err != nil {
		return err
	}
	return nil
}
