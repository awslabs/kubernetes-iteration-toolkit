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
	"net"
	"syscall"

	"github.com/aws/aws-sdk-go/aws/awserr"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	WaitingForSubResources = errors.New("waiting for subresources")
)

func IsNotFound(err error) bool {
	return kubeerrors.IsNotFound(err)
}

func IsWaitingForSubResource(err error) bool {
	return errors.Is(err, WaitingForSubResources)
}

func IsDNSLookUpNoSuchHost(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func IsNetIOTimeOut(err error) bool {
	netErr := net.Error(nil)
	return errors.As(err, &netErr) && netErr.Temporary() && netErr.Timeout()
}

func IsConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}

func IsLaunchTemplateDoNotExist(err error) bool {
	awsErr := awserr.Error(nil)
	return errors.As(err, &awsErr) && awsErr.Code() == "InvalidLaunchTemplateName.NotFoundException"
}
