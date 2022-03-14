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

package kubeprovider

import (
	"context"
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	client.Client
}

func New(client client.Client) *Client {
	return &Client{client}
}

// EnsureCreate creates the object if not exist, some of the objects can't be
// patched directly like (service, secrets). In case when patching service
// object for example, API server returns an error that clusterIP is immutable.
// So we ensure that it's always created and exists and is not patched. Will
// revisit both these to define what all users can change in an existing
// cluster.
func (c *Client) EnsureCreate(ctx context.Context, desired client.Object) error {
	existingObject := desired.DeepCopyObject().(client.Object)
	if err := c.Get(ctx, client.ObjectKeyFromObject(desired), existingObject); err != nil {
		if errors.IsNotFound(err) {
			return c.Create(ctx, desired)
		}
		return fmt.Errorf("getting object when creating %v, name %v, %w",
			desired.GetObjectKind().GroupVersionKind().GroupKind().String(), desired.GetName(), err)
	}
	return nil
}

// EnsurePatch creates if not exist, else will patch the existing object. Its
// used for deployments, statefulsets to provide configurability for flags.
func (c *Client) EnsurePatch(ctx context.Context, object, desired client.Object) error {
	if err := c.Get(ctx, client.ObjectKeyFromObject(desired), object); err != nil {
		if errors.IsNotFound(err) {
			return c.Create(ctx, desired)
		}
		return fmt.Errorf("getting object %v, name %v, %w",
			desired.GetObjectKind().GroupVersionKind().GroupKind().String(), desired.GetName(), err)
	}
	desired.SetResourceVersion(object.GetResourceVersion())
	if err := c.Patch(ctx, desired, client.StrategicMergeFrom(object)); err != nil {
		return fmt.Errorf("failed to patch, %v, %w", desired.GetName(), err)
	}
	return nil
}
