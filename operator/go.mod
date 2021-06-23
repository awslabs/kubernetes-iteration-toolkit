module github.com/awslabs/kit/operator

go 1.16

require (
	github.com/aws/aws-sdk-go v1.38.11
	github.com/awslabs/karpenter v0.2.2
	github.com/patrickmn/go-cache v2.1.0+incompatible
	go.uber.org/zap v1.16.0
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	k8s.io/api v0.19.7
	k8s.io/apimachinery v0.19.7
	k8s.io/client-go v0.19.7
	knative.dev/pkg v0.0.0-20210311174826-40488532be3f
	sigs.k8s.io/controller-runtime v0.7.0-alpha.3
)
