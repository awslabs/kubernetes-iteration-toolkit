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

	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/awsprovider/iam"
)

// reconcileAuthenticatorConfig creates required configs for aws-iam-authenticator and stores them as secret in api server
func (c *Controller) reconcileAuthenticatorConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	awsAccountID, err := c.cloudProvider.ID()
	if err != nil {
		return fmt.Errorf("getting AWS account ID, %w", err)
	}
	return c.awsIamAuthenticator.EnsureConfig(ctx, controlPlane.ClusterName(), controlPlane.Namespace,
		iam.KitNodeRoleNameFor(controlPlane.ClusterName()), awsAccountID)
}

func (c *Controller) reconcileAuthenticatorDaemonSet(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	return c.awsIamAuthenticator.EnsureDaemonSet(ctx, controlPlane, APIServerLabels(controlPlane.ClusterName()))
}
