package v1alpha1

import (
	"context"

	"knative.dev/pkg/apis"
)

func (s *Substrate) Validate(ctx context.Context) (errs *apis.FieldError) {
	if len(s.Name) == 0 {
		return errs.Also(apis.ErrMissingField("name"))
	}
	return nil
}
