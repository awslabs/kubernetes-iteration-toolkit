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

package functional

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

func ValidateAll(fns ...func() bool) bool {
	for _, fn := range fns {
		if !fn() {
			return false
		}
	}
	return true
}

func StringsMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	temp := make(map[string]struct{})
	for _, s := range a {
		temp[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := temp[s]; !ok {
			return false
		}
	}
	return true
}
