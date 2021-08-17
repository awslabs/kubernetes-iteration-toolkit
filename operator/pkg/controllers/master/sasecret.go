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

package master

import (
	"context"
	"fmt"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"
)

// reconcileSAKeyPair generate a key for API server to sign service accounts.
func (c *Controller) reconcileSAKeyPair(ctx context.Context, cp *v1alpha1.ControlPlane) error {
	secret, err := c.keypairs.GetOrGenerateSecret(ctx, &secrets.Request{
		Type:      secrets.KeyPair,
		Name:      saKeyPairSecretNameFor(cp.ClusterName()),
		Namespace: cp.Namespace,
	})
	if err != nil {
		return err
	}
	return c.kubeClient.EnsureCreate(ctx, object.WithOwner(cp, secret))
}

func saKeyPairSecretNameFor(clusterName string) string {
	return fmt.Sprintf("%s-sa-keypair", clusterName)
}
