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

package etcd

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func DefaultVolumeClaimTemplateSpec() []v1.PersistentVolumeClaim {
	storageClassName := "kit-gp3"
	return []v1.PersistentVolumeClaim{{
		ObjectMeta: metav1.ObjectMeta{
			Name: "etcd-data",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes:      []v1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			StorageClassName: &storageClassName,
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{"storage": resource.MustParse("40Gi")},
			},
		},
	}}

}
