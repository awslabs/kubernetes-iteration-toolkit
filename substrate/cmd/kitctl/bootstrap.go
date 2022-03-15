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
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/controller/substrate"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
)

func bootstrapCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap an environment for testing. Will reconnect if the environment already exists.",
		Long:  ``,
		Run:   bootstrap,
	}
}

func bootstrap(cmd *cobra.Command, args []string) {
	// ignore logs printed to stdout from underlying kubeadm packages
	if !options.debug {
		stdout := os.Stdout
		stderr := os.Stderr
		os.Stdout, _ = os.Open(os.DevNull)
		os.Stderr, _ = os.Open(os.DevNull)
		defer func() {
			os.Stdout = stdout
			os.Stderr = stderr
		}()
	}
	ctx := cmd.Context()
	start := time.Now()
	name := parseName(ctx, args)
	logging.FromContext(ctx).Infof("Bootstrapping %q", name)
	if err := substrate.NewController(ctx).Reconcile(ctx, &v1alpha1.Substrate{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.SubstrateSpec{
			VPC:          &v1alpha1.VPCSpec{CIDR: "10.0.0.0/16"},
			InstanceType: aws.String("r6g.large"),
			Subnets: []*v1alpha1.SubnetSpec{
				{Zone: "us-west-2a", CIDR: "10.0.1.0/24"},
				{Zone: "us-west-2b", CIDR: "10.0.2.0/24"},
				{Zone: "us-west-2c", CIDR: "10.0.3.0/24"},
				{Zone: "us-west-2a", CIDR: "10.0.100.0/24", Public: true},
				{Zone: "us-west-2b", CIDR: "10.0.101.0/24", Public: true},
				{Zone: "us-west-2c", CIDR: "10.0.102.0/24", Public: true},
			},
		},
	}); err != nil {
		logging.FromContext(ctx).Error(err.Error())
		return
	}
	logging.FromContext(ctx).Infof("✅ Bootstrapped %q after %s", name, time.Since(start))
}
