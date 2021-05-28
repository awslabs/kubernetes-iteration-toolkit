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

package resource

import (
	"github.com/aws/aws-sdk-go/aws"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NamespacedName(namespace, obj string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: obj}
}

func OwnerReference(o client.Object) []metav1.OwnerReference {
	return []metav1.OwnerReference{{
		APIVersion:         o.GetObjectKind().GroupVersionKind().Version,
		Kind:               o.GetObjectKind().GroupVersionKind().Kind,
		Name:               o.GetName(),
		UID:                o.GetUID(),
		BlockOwnerDeletion: aws.Bool(true),
	}}
}

func ObjectMeta(o client.Object) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            o.GetName(),
		Namespace:       o.GetNamespace(),
		OwnerReferences: OwnerReference(o),
	}
}
