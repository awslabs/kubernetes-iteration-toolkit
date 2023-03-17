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

package main

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/controller/substrate"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
)

func bootstrapCommand() *cobra.Command {
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap an environment for testing. Will reconnect if the environment already exists.",
		Long:  ``,
		Run:   bootstrap,
	}
	bootstrapCmd.Flags().StringP("instanceType", "i", "r6g.8xlarge", "Instance type for the substrate nodes.")
	return bootstrapCmd
}

func bootstrap(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	if debug {
		dev, err := zap.NewDevelopment()
		if err != nil {
			panic(err)
		}
		ctx = logging.WithLogger(ctx, dev.Sugar())
	}
	instanceType, err := cmd.Flags().GetString("instanceType")
	if err != nil {
		panic(err)
	}
	start := time.Now()
	name := parseName(ctx, args)
	logging.FromContext(ctx).Infof("Bootstrapping %q", name)
	if err := substrate.NewController(ctx).Reconcile(ctx, &v1alpha1.Substrate{
		ObjectMeta: metav1.ObjectMeta{Name: name},

		Spec: v1alpha1.SubstrateSpec{
			VPC:          &v1alpha1.VPCSpec{CIDR: []string{"10.0.0.0/16"}},
			InstanceType: aws.String(instanceType),
			Subnets: []*v1alpha1.SubnetSpec{
				{Zone: "us-west-2a", CIDR: "10.0.32.0/19"},
				{Zone: "us-west-2b", CIDR: "10.0.64.0/19"},
				{Zone: "us-west-2c", CIDR: "10.0.96.0/19"},
				{Zone: "us-west-2a", CIDR: "10.0.128.0/19", Public: true},
				{Zone: "us-west-2b", CIDR: "10.0.160.0/19", Public: true},
				{Zone: "us-west-2c", CIDR: "10.0.192.0/19", Public: true},
			},
		},
	}); err != nil {
		logging.FromContext(ctx).Error(err.Error())
		return
	}
	logging.FromContext(ctx).Infof("✅ Bootstrapped %q after %s", name, time.Since(start))
}
