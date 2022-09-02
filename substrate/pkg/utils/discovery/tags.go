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

package discovery

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	OwnerTagKey = "kit.aws/substrate"
)

func Tags(ownerName string, name *string) []*ec2.Tag {
	return []*ec2.Tag{
		{Key: aws.String(OwnerTagKey), Value: aws.String(ownerName)},
		{Key: aws.String("Name"), Value: name},
	}
}

func Filters(name string, optionalName ...*string) (filters []*ec2.Filter) {
	if len(optionalName) > 1 {
		panic("name cannot have more than one value")
	}
	filters = append(filters, &ec2.Filter{Name: aws.String(fmt.Sprintf("tag:%s", OwnerTagKey)), Values: []*string{aws.String(name)}})
	if len(optionalName) > 0 {
		filters = append(filters, &ec2.Filter{Name: aws.String(fmt.Sprintf("tag:%s", "Name")), Values: optionalName})
	}
	return filters
}
