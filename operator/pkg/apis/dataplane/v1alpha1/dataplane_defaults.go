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
)

const (
	// TODO https://github.com/awslabs/kubernetes-iteration-toolkit/issues/61
	defaultDNSClusterIP = "10.100.0.10"
)

func (c *DataPlane) SetDefaults(ctx context.Context) {
	c.Spec.SetDefaults(ctx)
}

// SetDefaults for the DataPlaneSpec, cascading to all subspecs
func (s *DataPlaneSpec) SetDefaults(ctx context.Context) {
	if s.AllocationStrategy == "" {
		s.AllocationStrategy = "lowest-price"
	}
	if len(s.InstanceTypes) == 0 {
		s.InstanceTypes = []string{"t2.xlarge", "t3.xlarge", "t3a.xlarge"}
	}
	if s.DNSClusterIP == "" {
		s.DNSClusterIP = defaultDNSClusterIP
	}
}
