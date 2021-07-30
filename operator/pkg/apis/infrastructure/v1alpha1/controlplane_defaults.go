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
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// SetDefaults for the ControlPlane, this gets called by the kit-webhook pod
// Nothing is set here to default as we don't want to change the controlPlane
// CRD instance object in Kubernetes. All the defaults are set while reconciling
// over ControlPlane object in the controllers.
func (c *ControlPlane) SetDefaults(ctx context.Context) {
	c.Spec.SetDefaults(ctx)
}

// SetDefaults for the ControlPlaneSpec, cascading to all subspecs
func (s *ControlPlaneSpec) SetDefaults(ctx context.Context) {}

// WithStaticDefaults
func (p *ControlPlaneSpec) WithStaticDefaults(defaultStaticSpec func() *ControlPlaneSpec) (_ ControlPlaneSpec, err error) {
	userConfig := p.DeepCopy()
	patchedCPSpec, err := defaultStaticSpec().Patch(userConfig)
	return patchedCPSpec, err
}

// Patch will overwrite the receiver with desired fields in the parameter provided
func (c *ControlPlaneSpec) Patch(patch *ControlPlaneSpec) (spec ControlPlaneSpec, err error) {
	if patch.KubernetesVersion != "" {
		c.KubernetesVersion = patch.KubernetesVersion
	}
	if c.Etcd, err = patch.Etcd.WithStaticDefaults(context.TODO(), func() *ETCDSpec {
		return &c.Etcd
	}); err != nil {
		return ControlPlaneSpec{}, err
	}
	return *c, err
}

func (s *ETCDSpec) WithStaticDefaults(ctx context.Context, defaultStaticSpec func() *ETCDSpec) (_ ETCDSpec, err error) {
	spec := s.DeepCopy()
	patchedEtcdSpec, err := defaultStaticSpec().Patch(spec)
	return patchedEtcdSpec, err
}

func (s *ETCDSpec) Patch(patch *ETCDSpec) (ETCDSpec, error) {
	if patch.Instances.AMI != "" {
		s.Instances.AMI = patch.Instances.AMI
	}
	if patch.Instances.Type != "" {
		s.Instances.Type = patch.Instances.Type
	}
	if patch.Spec != nil {
		obj := v1.PodSpec{}
		mergedPatch, err := mergePatch(s.Spec, patch.Spec, obj)
		if err != nil {
			return ETCDSpec{}, fmt.Errorf("failed to merge patch, %w", err)
		}
		if err := json.Unmarshal(mergedPatch, s.Spec); err != nil {
			return ETCDSpec{}, fmt.Errorf("unmarshalling mergedPatch to podSpec, %w", err)
		}
	}
	return *s, nil
}

func mergePatch(defaultObj, patch, object interface{}) ([]byte, error) {
	defaultSpecBytes, err := json.Marshal(defaultObj)
	if err != nil {
		return nil, err
	}
	patchSpecBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	patchedBytes, err := strategicpatch.StrategicMergePatch(defaultSpecBytes, patchSpecBytes, object)
	if err != nil {
		return nil, fmt.Errorf("json merge patch, %w", err)
	}
	return patchedBytes, nil
}
