package kiterr

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws/awserr"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	ErrWaitingForSubResources = errors.New("waiting for subresource to be ready")
)

func IsErrWaitingForSubResource(err error) bool {
	if err != nil {
		return errors.Is(err, ErrWaitingForSubResources)
	}
	return false
}

func IsErrSubnetExists(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == "InvalidSubnet.Conflict"
		}
	}
	return false
}

func IsErrDependencyExists(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == "DependencyViolation"
		}
	}
	return false
}

func IsErrWhileRemovingFinalizer(err error) bool {
	if k := kubeerrors.APIStatus(nil); errors.As(err, &k) {
		if k.Status().Reason == "Invalid" {
			return true
		}
	}
	return false
}
