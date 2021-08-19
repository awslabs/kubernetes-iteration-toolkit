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

package v1alpha1

import (
	"context"

	"github.com/awslabs/kit/operator/pkg/apis/config"
)

// SetDefaults for the ControlPlane, this gets called by the kit-webhook pod
// Nothing is set here to default as we don't want to change the controlPlane
// CRD instance object in Kubernetes. All the defaults are set while reconciling
// over ControlPlane object in the controllers.
func (c *ControlPlane) SetDefaults(ctx context.Context) {
	c.Spec.SetDefaults(ctx)
}

// SetDefaults for the ControlPlaneSpec, cascading to all subspecs
func (s *ControlPlaneSpec) SetDefaults(ctx context.Context) {
	if s.KubernetesVersion == "" {
		s.KubernetesVersion = config.DefaultKubernetesVersion
	}
	if s.Master.APIServer == nil {
		s.Master.APIServer = &Component{}
	}
}
