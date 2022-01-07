module github.com/awslabs/kit/substrate

go 1.16

replace github.com/awslabs/kit/operator => ../operator/

require (
	github.com/aws/aws-sdk-go v1.42.23
	github.com/awslabs/kit/operator v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.3.0
	go.uber.org/zap v1.19.1
	k8s.io/apimachinery v0.21.4
	k8s.io/client-go v0.21.4
	knative.dev/pkg v0.0.0-20211215065729-552319d4f55b
)
