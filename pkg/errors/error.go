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

package errors

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/iam"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	WaitingForSubResources = errors.New("waiting for subresource to be ready")
)

func SafeToIgnore(err error) bool {
	return IsWaitingForSubResource(err) ||
		IsDependencyExists(err) ||
		IsIAMResourceDependencyExists(err) ||
		IsTargetGroupResourceInUse(err) ||
		IsElasticIPInUse(err) ||
		IsASGWaitingForIAMResources(err)
}

func IsWaitingForSubResource(err error) bool {
	if err != nil {
		return errors.Is(err, WaitingForSubResources)
	}
	return false
}

func IsSubnetExists(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == "InvalidSubnet.Conflict"
		}
	}
	return false
}

func IsDependencyExists(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == "DependencyViolation"
		}
	}
	return false
}

func IsWhileRemovingFinalizer(err error) bool {
	if k := kubeerrors.APIStatus(nil); errors.As(err, &k) {
		if k.Status().Reason == "Invalid" {
			return true
		}
	}
	return false
}

func IsASGWaitingForIAMResources(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == "ValidationError"
		}
	}
	return false
}

func KubeObjNotFound(err error) bool {
	return kubeerrors.IsNotFound(err)
}

func IsIAMResourceNotFound(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == iam.ErrCodeNoSuchEntityException
		}
	}
	return false
}

func IsIAMResourceDependencyExists(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == iam.ErrCodeDeleteConflictException
		}
	}
	return false
}

func IsELBLoadBalancerNotExists(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == elbv2.ErrCodeLoadBalancerNotFoundException
		}
	}
	return false
}

func IsTargetGroupResourceInUse(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == elbv2.ErrCodeResourceInUseException
		}
	}
	return false
}

func IsElasticIPInUse(err error) bool {
	if err != nil && strings.Contains(err.Error(), "AuthFailure: You do not have permission to access the specified resource") {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == "AuthFailure"
		}
	}
	return false
}

func IsTargetGroupNotExists(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == elbv2.ErrCodeTargetGroupNotFoundException
		}
	}
	return false
}
