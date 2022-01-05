package v1alpha1

import (
	"context"

	"knative.dev/pkg/ptr"
)

// SetDefaults for the resource
func (s *Substrate) SetDefaults(ctx context.Context) {
	if s.Spec.InstanceType == nil {
		s.Spec.InstanceType = ptr.String("t4g.nano")
	}
}

// UNUSED
// func (s *Substrate) defaultName() {
// 	if s.Name != "" {
// 		return
// 	}
// 	if s.GenerateName != "" {
// 		s.GenerateName = env.WithDefaultString("USER", strings.ToLower(randomdata.SillyName()))
// 	}
// 	s.Name = fmt.Sprintf("%s-%s", s.GenerateName, strings.ToLower(randomdata.SillyName()))
// }
