package main

import (
	"flag"
	"fmt"

	"github.com/awslabs/kit/operator/pkg/controllers"
	"github.com/awslabs/kit/operator/pkg/controllers/controlplane"
	"github.com/awslabs/kit/operator/pkg/utils/scheme"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	controllerruntime "sigs.k8s.io/controller-runtime"
	controllerruntimezap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	options = Options{}
)

// Options for running this binary
type Options struct {
	EnableVerboseLogging bool
	MetricsPort          int
	WebhookPort          int
}

func main() {
	flag.BoolVar(&options.EnableVerboseLogging, "verbose", false, "Enable verbose logging")
	flag.IntVar(&options.WebhookPort, "webhook-port", 9443, "The port the webhook endpoint binds to for validation and mutation of resources")
	flag.IntVar(&options.MetricsPort, "metrics-port", 8080, "The port the metric endpoint binds to for operating metrics about the controller itself")
	flag.Parse()

	logger := controllerruntimezap.NewRaw(controllerruntimezap.UseDevMode(options.EnableVerboseLogging),
		controllerruntimezap.ConsoleEncoder(),
		controllerruntimezap.StacktraceLevel(zapcore.DPanicLevel))
	controllerruntime.SetLogger(zapr.NewLogger(logger))
	zap.ReplaceGlobals(logger)

	manager := controllers.NewManagerOrDie(controllerruntime.GetConfigOrDie(), controllerruntime.Options{
		LeaderElection:          true,
		LeaderElectionID:        "kit-leader-election",
		Scheme:                  scheme.SubstrateCluster,
		MetricsBindAddress:      fmt.Sprintf(":%d", options.MetricsPort),
		Port:                    options.WebhookPort,
		LeaderElectionNamespace: "kit",
	})

	err := manager.RegisterControllers(
		controlplane.NewController(manager.GetClient())).Start(controllerruntime.SetupSignalHandler())
	if err != nil {
		panic(fmt.Sprintf("Unable to start manager, %v", err))
	}
}
