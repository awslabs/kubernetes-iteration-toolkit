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

package awsprovider

import (
	"fmt"
	"os"

	"github.com/awslabs/kit/operator/pkg/utils/project"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
)

func NewSession() *session.Session {
	return withUserAgent(withRegion(session.Must(
		session.NewSession(
			&aws.Config{STSRegionalEndpoint: endpoints.RegionalSTSEndpoint},
		))),
	)
}

func withRegion(sess *session.Session) *session.Session {
	region := os.Getenv("AWS_REGION")
	var err error
	if region == "" {
		region, err = ec2metadata.New(sess).Region()
		if err != nil {
			panic(fmt.Sprintf("failed to call the metadata server's region API, %v", err))
		}
	}
	sess.Config.Region = aws.String(region)
	return sess
}

// withUserAgent adds a kit specific user-agent string to AWS session
func withUserAgent(sess *session.Session) *session.Session {
	userAgent := fmt.Sprintf("kit.sh-%s", project.Version)
	sess.Handlers.Build.PushBack(request.MakeAddToUserAgentFreeFormHandler(userAgent))
	return sess
}

type EC2 struct {
	*ec2.EC2
}

func EC2Client(sess *session.Session) *EC2 {
	return &EC2{EC2: ec2.New(sess)}
}

type SSM struct {
	ssmiface.SSMAPI
}

func SSMClient(sess *session.Session) *SSM {
	return &SSM{SSMAPI: ssm.New(sess)}
}

type AutoScaling struct {
	*autoscaling.AutoScaling
}

func AutoScalingClient(sess *session.Session) *AutoScaling {
	return &AutoScaling{AutoScaling: autoscaling.New(sess)}
}

type IAM struct {
	*iam.IAM
}

func IAMClient(sess *session.Session) *IAM {
	return &IAM{IAM: iam.New(sess)}
}

type AccountMetadata interface {
	ID() (string, error)
}

type AccountInfo struct {
	Session *session.Session
}

func (a *AccountInfo) ID() (string, error) {
	doc, err := ec2metadata.New(a.Session).GetInstanceIdentityDocument()
	if err != nil {
		return "", fmt.Errorf("getting instance metadata, %v", err)
	}
	return doc.AccountID, nil
}
