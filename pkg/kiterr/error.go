package kiterr

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	WaitingForSubResources = errors.New("waiting for subresource to be ready")
)

func SafeToIgnore(err error) bool {
	return IsWaitingForSubResource(err) ||
		IsDependencyExists(err) ||
		IsWhileRemovingFinalizer(err)
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

func IsErrIAMResourceNotFound(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == iam.ErrCodeNoSuchEntityException
		}
	}
	return false
}

func IsErrIAMResourceDependencyExists(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == iam.ErrCodeDeleteConflictException
		}
	}
	return false
}
