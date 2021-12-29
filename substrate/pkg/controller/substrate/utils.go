package substrate

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
)

const (
	TagKeyNameForAWSResources = "kit.k8s.sh/substrate"
)

func generateEC2Tags(svcName, identifier string) []*ec2.TagSpecification {
	return []*ec2.TagSpecification{{
		ResourceType: aws.String(svcName),
		Tags: []*ec2.Tag{{
			Key:   aws.String(TagKeyNameForAWSResources),
			Value: aws.String(identifier),
		}, {
			Key:   aws.String("Name"),
			Value: aws.String(identifier),
		}},
	}}
}

func generateEC2TagsWithName(svcName, identifier, name string) []*ec2.TagSpecification {
	return []*ec2.TagSpecification{{
		ResourceType: aws.String(svcName),
		Tags: []*ec2.Tag{{
			Key:   aws.String(TagKeyNameForAWSResources),
			Value: aws.String(identifier),
		}, {
			Key:   aws.String("Name"),
			Value: aws.String(name),
		}},
	}}
}

func ec2FilterFor(identifier string) []*ec2.Filter {
	return []*ec2.Filter{{
		Name:   aws.String(fmt.Sprintf("tag:%s", TagKeyNameForAWSResources)),
		Values: []*string{aws.String(identifier)},
	}}
}

func iamResourceNotFound(err error) bool {
	if aerr := awserr.Error(nil); errors.As(err, &aerr) {
		if aerr.Code() == iam.ErrCodeNoSuchEntityException {
			return true
		}
	}
	return false
}
