package main

import (
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
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
	runtime.Must(substrate.NewController(cmd.Context()).Finalize(cmd.Context(), &v1alpha1.Substrate{
		ObjectMeta: metav1.ObjectMeta{Name: "testvpc"},
		Spec: v1alpha1.SubstrateSpec{
			VPC: &v1alpha1.VPCSpec{CIDR: "10.0.0.0/16"},
		},
	}))
}
