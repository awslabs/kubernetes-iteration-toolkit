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
	"fmt"
	"os/user"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/util/runtime"
	"knative.dev/pkg/logging"
)

func parseName(ctx context.Context, args []string) string {
	if len(args) > 1 {
		logging.FromContext(ctx).Fatalf("Too many args provided expected only 1 %v", args)
	}
	if len(args) == 1 {
		return args[0]
	}
	// default name if not provided
	u, err := user.Current()
	runtime.Must(err)
	return fmt.Sprintf("kitctl-%s", u.Username)
}

func productionZapLogger() *zap.Logger {
	config := zap.NewProductionConfig()
	config.Encoding = "console"
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("Jan 02 15:04:05.000000000")
	config.EncoderConfig.StacktraceKey = "" // to hide stacktrace info
	logger, err := config.Build()
	if err != nil {
		panic(err)
	}
	return logger
}

func developmentZapLogger() *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("Jan 02 15:04:05.000000000")
	config.EncoderConfig.StacktraceKey = "" // to hide stacktrace info
	logger, err := config.Build()
	if err != nil {
		panic(err)
	}
	return logger
}
