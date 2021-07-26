module github.com/awslabs/kit/operator

go 1.16

require (
	github.com/aws/aws-sdk-go v1.38.62
	github.com/awslabs/karpenter v0.2.8
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/pkg/errors v0.9.1
	go.uber.org/zap v1.17.0
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	k8s.io/api v0.20.7
	k8s.io/apimachinery v0.20.7
	k8s.io/client-go v0.20.7
	knative.dev/pkg v0.0.0-20210628225612-51cfaabbcdf6
	sigs.k8s.io/controller-runtime v0.8.3
)
