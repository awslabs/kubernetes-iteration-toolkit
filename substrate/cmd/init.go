package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"knative.dev/pkg/logging"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize and connect to an environment for testing. Will reconnect if the environment already exists.",
	Long:  ``,
	Run:   Init,
}

func Init(cmd *cobra.Command, args []string) {

	data := []byte{}

	// os.Open()
	// if options.File == "" {
	// 	file := os.Stdin
	// }

	_ = File(cmd.Context())

	// b1 := make([]byte, 5)
	// n1, err := f.Read(b1)
	// check(err)
	// fmt.Printf("%d bytes: %s\n", n1, string(b1[:n1]))

	logging.FromContext(cmd.Context()).Info(data)
	// substrate.NewController().Reconcile(ctx, substrate)
}

func File(ctx context.Context) *os.File {
	if options.File == "" {
		return os.Stdin
	}
	file, err := os.Open(options.File)
	if err != nil {
		logging.FromContext(ctx).Fatal(err)
	}
	return file
}

func init() {
	rootCmd.AddCommand(initCmd)
}
