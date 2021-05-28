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

	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Subnet struct {
	KubeClient client.Client
	Region     string
}

func (s *Subnet) Create(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	if err := s.exists(ctx, controlPlane.Namespace, controlPlane.Name); err != nil {
		if errors.IsNotFound(err) {
			if err := s.create(ctx, controlPlane); err != nil {
				return fmt.Errorf("creating kube object, %w", err)
			}
			return nil
		}
		return fmt.Errorf("getting Subnet object, %w", err)
	}
	// TODO verify existing matches the desired else update the object
	return nil
}

func (s *Subnet) create(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// Create Subnet object
	items := s.subnetProperties(controlPlane)
	if err := s.KubeClient.Create(ctx, &v1alpha1.Subnet{
		ObjectMeta: ObjectMeta(controlPlane, ""),
		Spec: v1alpha1.SubnetSpec{
			Items: items,
		},
	}); err != nil {
		return fmt.Errorf("creating subnet kube object, %w", err)
	}
	zap.S().Debugf("Successfully created Subnet object for cluster %v", controlPlane.Name)
	return nil
}

func (s *Subnet) subnetProperties(controlPlane *v1alpha1.ControlPlane) []*v1alpha1.SubnetProperty {
	// TODO hardcoded subnets for now, make defaults
	privateSubnetCIDRs := []string{"10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"}
	publicSubnetCIDRs := []string{"10.0.11.0/24", "10.0.12.0/24", "10.0.13.0/24"}
	result := []*v1alpha1.SubnetProperty{}
	for i, zone := range s.availabilityZonesForRegion(s.Region) {
		result = append(result,
			&v1alpha1.SubnetProperty{
				CIDR:   privateSubnetCIDRs[i],
				AZ:     zone,
				Public: false,
			},
			&v1alpha1.SubnetProperty{
				CIDR:   publicSubnetCIDRs[i],
				AZ:     zone,
				Public: true,
			},
		)
	}
	return result
}

// TODO get all AZs for a region from an API
func (s *Subnet) availabilityZonesForRegion(region string) []string {
	azs := []string{}
	for _, azPrefix := range []string{"a", "b", "c"} {
		azs = append(azs, fmt.Sprintf(region+azPrefix))
	}
	return azs
}

func (s *Subnet) exists(ctx context.Context, ns, objName string) error {
	result := &v1alpha1.Subnet{}
	if err := s.KubeClient.Get(ctx, NamespacedName(ns, objName), result); err != nil {
		return err
	}
	return nil
}
