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

	"github.com/awslabs/kit/operator/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	client.Client
}

func New(client client.Client) *Client {
	return &Client{client}
}

// Ensure creates if not exist, else will update the existing object
func (c *Client) Ensure(ctx context.Context, desired client.Object) error {
	existingObject := desired.DeepCopyObject().(client.Object)
	if err := c.Get(ctx, client.ObjectKeyFromObject(desired), existingObject); err != nil {
		if errors.IsNotFound(err) {
			return c.Create(ctx, desired)
		}
		return fmt.Errorf("getting object %v, name %v, %w",
			desired.GetObjectKind().GroupVersionKind().GroupKind().String(), desired.GetName(), err)
	}
	if err := c.Patch(ctx, existingObject, client.StrategicMergeFrom(desired)); err != nil {
		return fmt.Errorf("failed to patch, %v, %w", desired.GetName(), err)
	}
	return nil
}
