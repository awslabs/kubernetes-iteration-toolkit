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

package patch

import (
	"encoding/json"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// PodSpec will merge the patch with the default pod spec and return the merged podSpec object
func PodSpec(defaultSpec, patch *v1.PodSpec) (v1.PodSpec, error) {
	if patch == nil {
		return *defaultSpec, nil
	}
	obj := v1.PodSpec{}
	mergedPatch, err := mergePatch(defaultSpec, mergeContainerArgs(defaultSpec, patch), obj)
	if err != nil {
		return v1.PodSpec{}, err
	}
	result := &v1.PodSpec{}
	if err := json.Unmarshal(mergedPatch, result); err != nil {
		return v1.PodSpec{}, fmt.Errorf("unmarshalling merged patch to podSpec, %w", err)
	}
	return *result, nil
}

func mergePatch(defaultObj, patch, object interface{}) ([]byte, error) {
	defaultSpecBytes, err := json.Marshal(defaultObj)
	if err != nil {
		return nil, err
	}
	patchSpecBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	patchedBytes, err := strategicpatch.StrategicMergePatch(defaultSpecBytes, patchSpecBytes, object)
	if err != nil {
		return nil, fmt.Errorf("json merge patch, %w", err)
	}
	return patchedBytes, nil
}

func mergeContainerArgs(defaultSpec, patch *v1.PodSpec) *v1.PodSpec {
	merged := []string{}
	for key, value := range UnionStringMaps(parseArgsFor(defaultSpec), parseArgsFor(patch)) {
		merged = append(merged, strings.Join([]string{key, value}, "="))
	}
	patch.Containers[0].Args = merged
	return patch
}

func parseArgsFor(podSpec *v1.PodSpec) map[string]string {
	result := map[string]string{}
	for _, arg := range podSpec.Containers[0].Args {
		kv := strings.Split(arg, "=")
		result[kv[0]] = result[kv[1]]
	}
	return result
}

func UnionStringMaps(dest, src map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range dest {
		result[key] = value
	}
	for key, value := range src {
		result[key] = value
	}
	return result
}
