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

package object

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ControlPlaneLabelKey = v1alpha1.SchemeGroupVersion.Group + "/control-plane-name"
	AppNameLabelKey      = v1alpha1.SchemeGroupVersion.Group + "/app"
)

func WithOwner(owner, obj client.Object) client.Object {
	obj.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: owner.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Name:       owner.GetName(),
		Kind:       owner.GetObjectKind().GroupVersionKind().Kind,
		UID:        owner.GetUID(),
	}})
	return obj
}

func NamespacedName(name, namespace string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

func GenerateConfigMap(strtmpl string, obj interface{}) (*v1.ConfigMap, error) {
	var buf bytes.Buffer
	tmpl := template.Must(template.New("Text").Parse(strtmpl))
	err := tmpl.Execute(&buf, obj)
	if err != nil {
		return nil, fmt.Errorf("error when executing template, %w", err)
	}
	configMap := &v1.ConfigMap{}
	err = kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), buf.Bytes(), configMap)
	return configMap, err
}
