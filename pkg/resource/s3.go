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

type S3 struct {
	KubeClient client.Client
}

func (s *S3) Create(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	if err := s.exists(ctx, controlPlane.Namespace, controlPlane.Name); err != nil {
		if errors.IsNotFound(err) {
			if err := s.create(ctx, controlPlane); err != nil {
				return fmt.Errorf("creating kube object, %w", err)
			}
			return nil
		}
		return fmt.Errorf("getting S3 object, %w", err)
	}
	// TODO verify existing object matches the desired else update
	return nil
}

func (s *S3) create(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	if err := s.KubeClient.Create(ctx, &v1alpha1.S3{
		ObjectMeta: ObjectMeta(controlPlane, ""),
		Spec: v1alpha1.S3Spec{
			BucketName: fmt.Sprintf("kit-%s", controlPlane.Name), // TODO add random ID
		},
	}); err != nil {
		return fmt.Errorf("creating kube object, %w", err)
	}
	zap.S().Debugf("Successfully created S3 object for cluster %v", controlPlane.Name)
	return nil
}

func (s *S3) exists(ctx context.Context, ns, objName string) error {
	result := &v1alpha1.S3{}
	if err := s.KubeClient.Get(ctx, NamespacedName(ns, objName), result); err != nil {
		return err
	}
	return nil
}
