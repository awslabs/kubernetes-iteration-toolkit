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

package substrate

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	OwnerTagKey = "kit.k8s.sh/substrate"
)

func tagsFor(svcName, owner string, name string) []*ec2.TagSpecification {
	return []*ec2.TagSpecification{{
		ResourceType: aws.String(svcName),
		Tags: []*ec2.Tag{
			{Key: aws.String(OwnerTagKey), Value: aws.String(owner)},
			{Key: aws.String("Name"), Value: aws.String(name)},
		},
	}}
}

func filtersFor(owner string, name ...string) (filters []*ec2.Filter) {
	if len(name) > 1 {
		panic("name cannot have more than one value")
	}
	filters = append(filters, &ec2.Filter{Name: aws.String(fmt.Sprintf("tag:%s", OwnerTagKey)), Values: []*string{aws.String(owner)}})
	if len(name) > 0 {
		filters = append(filters, &ec2.Filter{Name: aws.String(fmt.Sprintf("tag:%s", "Name")), Values: aws.StringSlice(name)})
	}
	return filters
}
