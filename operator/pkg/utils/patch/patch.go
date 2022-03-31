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

var blockListedFlags = map[string]map[string]bool{
	"etcd": {
		"--initial-cluster-state": true,
	},
}

// VolumeClaimTemplateSpec returns the merged VolumeClaimTemplate spes
func PersistentVolumeClaimSpec(defaultSpec, patch *v1.PersistentVolumeClaimSpec) (v1.PersistentVolumeClaimSpec, error) {
	if patch == nil {
		return *defaultSpec, nil
	}
	merged, err := mergePatch(defaultSpec, patch, v1.PersistentVolumeClaimSpec{})
	if err != nil {
		return v1.PersistentVolumeClaimSpec{}, err
	}
	result := &v1.PersistentVolumeClaimSpec{}

	if err := json.Unmarshal(merged, result); err != nil {
		return v1.PersistentVolumeClaimSpec{}, fmt.Errorf("unmarshalling merged patch to persistentVolumeClaimSpec, %w", err)
	}
	return *result, nil
}

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

// Keep the order of the args same, if the ordering changes when object is patched Kubernetes restarts the pod
func mergeContainerArgs(defaultSpec, patch *v1.PodSpec) *v1.PodSpec {
	patchedArgs := parseArgsFor(patch)
	// get any additional args passed in patch
	extraArgs := additionalArgs(parseArgsFor(defaultSpec), patch)
	updatedArgs := []string{}
	// for all the args in defaultSpec, check if the value for an arg has been updated in patch
	for _, arg := range defaultSpec.Containers[0].Args {
		kv := strings.Split(arg, "=")
		if update, ok := patchedArgs[kv[0]]; ok {
			kv[1] = update
		}
		updatedArgs = append(updatedArgs, strings.Join(kv, "="))
	}
	patch.Containers[0].Args = append(updatedArgs, extraArgs...)
	return patch
}

func parseArgsFor(podSpec *v1.PodSpec) map[string]string {
	result := map[string]string{}
	for _, arg := range podSpec.Containers[0].Args {
		if strings.Contains(arg, "=") {
			kv := strings.Split(arg, "=")
			result[kv[0]] = kv[1]
		}
	}
	return result
}

// needs to preserve the order of args passed in patch in every iteration
func additionalArgs(defaultSpec map[string]string, patch *v1.PodSpec) []string {
	result := make([]string, 0)
	for _, arg := range patch.Containers[0].Args {
		kv := strings.Split(arg, "=")
		_, ok := defaultSpec[kv[0]]
		if !ok && !blockListedFlags[patch.Containers[0].Name][kv[0]] {
			result = append(result, arg)
		}
	}
	return result
}
