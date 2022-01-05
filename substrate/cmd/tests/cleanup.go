package main

import (
	"context"

	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	err := substrate.Finalize(context.Background(), &v1alpha1.Substrate{
		ObjectMeta: metav1.ObjectMeta{Name: "testvpc"},
		Spec: v1alpha1.SubstrateSpec{
			VPC: &v1alpha1.VPCSpec{CIDR: "10.0.0.0/16"},
		},
	})
	if err != nil {
		panic(err)
	}
}
