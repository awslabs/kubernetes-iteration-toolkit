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

package etcd

import (
	"context"
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	"go.uber.org/multierr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/ptr"
)

func (c *Controller) reconcilePersistentVolumeClaims(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (errs error) {
	for i := 0; i < controlPlane.Spec.Etcd.Replicas; i++ {
		pvc := &v1.PersistentVolumeClaim{}
		pvcKey := types.NamespacedName{Namespace: controlPlane.Namespace, Name: fmt.Sprintf("etcd-data-%s-%d", ServiceNameFor(controlPlane.ClusterName()), i)}
		if err := c.kubeClient.Get(ctx, pvcKey, pvc); err != nil {
			if !errors.IsNotFound(err) {
				errs = multierr.Append(errs, fmt.Errorf("getting pvc %s, %w", pvcKey.Name, err))
			}
			continue
		}
		if err := c.kubeClient.Update(ctx, object.WithOwner(controlPlane, pvc)); err != nil {
			errs = multierr.Append(errs, fmt.Errorf("failed to update pvc %s, %w", pvcKey.Name, err))
		}
	}
	return errs
}

func DefaultPersistentVolumeClaimSpec() *v1.PersistentVolumeClaimSpec {
	return &v1.PersistentVolumeClaimSpec{
		AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
		StorageClassName: ptr.String("kit-gp3"),
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{"storage": resource.MustParse("40Gi")},
		},
	}
}
