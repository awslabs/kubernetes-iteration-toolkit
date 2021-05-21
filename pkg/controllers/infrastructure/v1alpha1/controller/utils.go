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

package controller

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	TagKeyNameForAWSResources = "kit.k8s.amazonaws.com/cluster-name"
	vpcCIDR                   = "10.0.0.0/16" // TODO hardcoded for now, make defaults
)

func generateEC2Tags(svcName, clusterName string) []*ec2.TagSpecification {
	return []*ec2.TagSpecification{{
		ResourceType: aws.String(svcName),
		Tags: []*ec2.Tag{{
			Key:   aws.String(TagKeyNameForAWSResources),
			Value: aws.String(clusterName),
		}, {
			Key:   aws.String("Name"),
			Value: aws.String(fmt.Sprintf("%s-%s", clusterName, svcName)),
		}},
	}}
}

func ec2FilterFor(clusterName string) []*ec2.Filter {
	return []*ec2.Filter{{
		Name:   aws.String(fmt.Sprintf("tag:%s", TagKeyNameForAWSResources)),
		Values: []*string{aws.String(clusterName)},
	}}
}

// TODO fix this naming
func generateAutoScalingTags(svcName, clusterName string) []*autoscaling.Tag {
	return []*autoscaling.Tag{
		&autoscaling.Tag{
			Key:               aws.String(TagKeyNameForAWSResources),
			Value:             aws.String(clusterName),
			PropagateAtLaunch: aws.Bool(true),
		},
		&autoscaling.Tag{
			Key:               aws.String("Name"),
			Value:             aws.String(svcName),
			PropagateAtLaunch: aws.Bool(true),
		},
	}
}

// TODO get all AZs for a region from an API
func availabilityZonesForRegion(region string) []string {
	azs := []string{}
	for _, azPrefix := range []string{"a", "b", "c"} {
		azs = append(azs, fmt.Sprintf(region+azPrefix))
	}
	return azs
}
