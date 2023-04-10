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
	"context"
	"math/rand"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/runtime"
	"knative.dev/pkg/logging"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rand.Seed(time.Now().UnixNano())
	runtime.Must(rootCmd.ExecuteContext(logging.WithLogger(ctx, productionZapLogger().Sugar())))
}

var (
	debug   bool
	rootCmd = &cobra.Command{
		Use:   "kitctl",
		Short: "A tool to provision Kubernetes cluster using kit-operator",
		Long: `kitctl help users provision Kubernetes clustes using kit-operator.
		It also configures the cloud provider environment to get started easily`,
	}
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "", false, "enable debug logs")
	// Add subcommands
	rootCmd.AddCommand(bootstrapCommand())
	rootCmd.AddCommand(deleteCommand())
}
