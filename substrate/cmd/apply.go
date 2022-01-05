package main

import (
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply an environment for testing. Will reconnect if the environment already exists.",
		Long:  ``,
		Run:   Apply,
	})
}

func Apply(cmd *cobra.Command, args []string) {
	// data := []byte{}
	// os.Open()
	// if options.File == "" {
	// 	file := os.Stdin
	// }
	// _ = File(cmd.Context())
	runtime.Must(substrate.NewController(cmd.Context()).Reconcile(cmd.Context(), &v1alpha1.Substrate{
		ObjectMeta: metav1.ObjectMeta{Name: "testvpc"},
		Spec: v1alpha1.SubstrateSpec{
			VPC: &v1alpha1.VPCSpec{CIDR: "10.0.0.0/16"},
		},
	}))
}

// func File(ctx context.Context) *os.File {
// 	if options.File == "" {
// 		return os.Stdin
// 	}
// 	file, err := os.Open(options.File)
// 	if err != nil {
// 		logging.FromContext(ctx).Fatal(err)
// 	}
// 	return file
// }
