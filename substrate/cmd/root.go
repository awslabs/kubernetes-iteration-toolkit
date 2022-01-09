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

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/runtime"
	"knative.dev/pkg/logging"
)

func main() {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment(zap.WithCaller(false))
	ctx = logging.WithLogger(ctx, logger.Sugar())
	runtime.Must(rootCmd.ExecuteContext(ctx))
}

var rootCmd = &cobra.Command{
	Use:   "kitctl",
	Short: "A tool to provision Kubernetes cluster using kit-operator",
	Long: `kitctl help users provision Kubernetes clustes using kit-operator.
It also supports configuring the cloud provider environment to get started easily`,
}

var options = Options{}

type Options struct {
	File string
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&options.File, "file", "f", "", "Configuration file for the environment")
}
