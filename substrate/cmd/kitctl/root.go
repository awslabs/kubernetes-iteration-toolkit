/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

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
	"context"
	"fmt"
	"io"
	"os"
	"os/user"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/util/runtime"
	"knative.dev/pkg/logging"
)

func main() {
	rootCmd := newRootCmd(os.Args[1:])
	logLevel := zapcore.InfoLevel
	if options.debug {
		logLevel = zapcore.DebugLevel
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := zap.New(zapcore.NewCore(zapcore.NewConsoleEncoder(zapcore.EncoderConfig{MessageKey: "message"}),
		customLogWriteTo(ctx, os.Stdout), zap.LevelEnablerFunc(func(level zapcore.Level) bool {
			return level >= logLevel
		}),
	))
	runtime.Must(rootCmd.ExecuteContext(logging.WithLogger(ctx, logger.Sugar())))
}

func newRootCmd(args []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kitctl",
		Short: "A tool to provision Kubernetes cluster using kit-operator",
		Long: `kitctl help users provision Kubernetes clustes using kit-operator.
		It also configures the cloud provider environment to get started easily`,
	}
	flags := cmd.PersistentFlags()
	options.addFlags(flags)
	if err := flags.Parse(args); err != nil {
		panic(err)
	}
	// Add subcommands
	cmd.AddCommand(bootstrapCommand())
	cmd.AddCommand(deleteCommand())
	return cmd
}

var options = &Options{}

type Options struct {
	file          string
	debug         bool
	substrateName string
	help          bool
}

func (o *Options) addFlags(fs *pflag.FlagSet) {
	u, err := user.Current()
	runtime.Must(err)
	fs.StringVarP(&o.file, "file", "f", "", "Configuration file for the environment")
	fs.StringVarP(&o.substrateName, "name", "n", fmt.Sprintf("kitctl-%s", u.Username), "name for the environment")
	fs.BoolVarP(&o.debug, "debug", "", false, "enable debug logs")
	fs.BoolVar(&o.help, "help", false, "help flag")
}

func customLogWriteTo(ctx context.Context, w io.Writer) *os.File {
	reader, writer, err := os.Pipe()
	if err != nil {
		panic(fmt.Sprintf("failed to create pipe, %v", err))
	}
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_, err := io.Copy(w, reader)
				if err != nil {
					panic(err)
				}
			}
		}
	}(ctx)
	return writer
}
