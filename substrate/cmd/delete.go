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

	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
)

func init() {
	rootCmd.LocalFlags().StringVar(&options.File, "name", "", "Name for the environment")
	rootCmd.LocalFlags().StringVarP(&options.File, "file", "f", "", "Configuration file for the environment")
	rootCmd.AddCommand(&cobra.Command{
		Use:   "delete",
		Short: "Delete the environment",
		Long:  ``,
		Run:   Delete,
	})
}

func Delete(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()
	start := time.Now()
	name := "test-substrate"
	if err := substrate.NewController(ctx).Reconcile(ctx, &v1alpha1.Substrate{
		ObjectMeta: metav1.ObjectMeta{Name: name, DeletionTimestamp: &metav1.Time{Time: time.Now()}},
		Spec: v1alpha1.SubstrateSpec{
			VPC: &v1alpha1.VPCSpec{CIDR: "10.0.0.0/16"},
		},
	}); err != nil {
		logging.FromContext(ctx).Error(err.Error())
		return
	}
	logging.FromContext(ctx).Infof("Deleted substrate %s after %s", name, time.Since(start))
}
