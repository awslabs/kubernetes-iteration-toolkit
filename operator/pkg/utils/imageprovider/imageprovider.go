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

package imageprovider

var (
	imageTags = map[string]string{
		"1.19": kubeVersion119Tag,
		"1.20": kubeVersion120Tag,
		"1.21": kubeVersion121Tag,
	}
)

func IsKubeVersionSupported(version string) bool {
	_, ok := imageTags[version]
	return ok
}

const (
	kubeVersion119Tag = "v1.19.13-eks-1-19-9"
	kubeVersion120Tag = "v1.20.7-eks-1-20-6"
	kubeVersion121Tag = "v1.21.2-eks-1-21-4"
	repositoryName    = "public.ecr.aws/eks-distro/"
	busyBoxImage      = "public.ecr.aws/docker/library/busybox:stable"
)

func APIServer(version string) string {
	return repositoryName + "kubernetes/kube-apiserver:" + imageTags[version]
}

func KubeControllerManager(version string) string {
	return repositoryName + "kubernetes/kube-controller-manager:" + imageTags[version]
}

func KubeScheduler(version string) string {
	return repositoryName + "kubernetes/kube-scheduler:" + imageTags[version]
}

func KubeProxy(version string) string {
	return repositoryName + "kubernetes/kube-proxy:" + imageTags[version]
}

func ETCD() string {
	return repositoryName + "etcd-io/etcd:v3.4.16-eks-1-21-4"
}

func CoreDNS() string {
	return repositoryName + "coredns/coredns:v1.8.3-eks-1-20-4"
}

func AWSIamAuthenticator() string {
	return repositoryName + "kubernetes-sigs/aws-iam-authenticator:v0.5.3-eks-1-21-8"
}

func AWSEncryptionProvider() string {
	return "public.ecr.aws/kit/aws-encryption-provider:0.0.1"
}

func BusyBox() string {
	return busyBoxImage
}
