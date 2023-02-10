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
		"1.22": kubeVersion122Tag,
		"1.23": kubeVersion123Tag,
		"1.24": kubeVersion124Tag,
		"1.25": kubeVersion125Tag,
	}
)

func IsKubeVersionSupported(version string) bool {
	_, ok := imageTags[version]
	return ok
}

// image tags come from EKS-D and are updated according to https://github.com/aws/eks-distro/issues/1174#issuecomment-1295638015
const (
	kubeVersion119Tag = "v1.19.16-eks-1-19-22"
	kubeVersion120Tag = "v1.20.15-eks-1-20-22"
	kubeVersion121Tag = "v1.21.14-eks-1-21-21"
	kubeVersion122Tag = "v1.22.16-eks-1-22-14"
	kubeVersion123Tag = "v1.23.13-eks-1-23-9"
	kubeVersion124Tag = "v1.24.8-eks-1-24-5"
	kubeVersion125Tag = "v1.25.5-eks-1-25-3"
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
	return repositoryName + "coredns/coredns:v1.8.7-eks-1-23-9"
}

func AWSIamAuthenticator() string {
	return repositoryName + "kubernetes-sigs/aws-iam-authenticator:v0.5.10-eks-1-23-9"
}

func AWSEncryptionProvider() string {
	// TODO update this to released version
	return "public.ecr.aws/kit/aws-encryption-provider:0.0.1"
}

func BusyBox() string {
	return busyBoxImage
}
