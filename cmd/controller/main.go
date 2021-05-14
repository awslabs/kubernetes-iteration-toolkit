package main

import (
	"flag"
	"fmt"

	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	infra "github.com/prateekgogia/kit/pkg/controllers/infrastructure/v1alpha1/controller"

	"github.com/awslabs/karpenter/pkg/utils/log"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	controllerruntimezap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme  = runtime.NewScheme()
	options = Options{}
)

func init() {
	log.PanicIfError(clientgoscheme.AddToScheme(scheme), "adding clientgo to scheme")
	log.PanicIfError(v1alpha1.AddToScheme(scheme), "adding cluster apis to scheme")
}

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

	log.Setup(
		controllerruntimezap.UseDevMode(options.EnableVerboseLogging),
		controllerruntimezap.ConsoleEncoder(),
		controllerruntimezap.StacktraceLevel(zapcore.DPanicLevel),
	)
	manager := controllers.NewManagerOrDie(controllerruntime.GetConfigOrDie(), controllerruntime.Options{
		LeaderElection:          true,
		LeaderElectionID:        "kit-leader-election",
		Scheme:                  scheme,
		MetricsBindAddress:      fmt.Sprintf(":%d", options.MetricsPort),
		Port:                    options.WebhookPort,
		LeaderElectionNamespace: "kit",
	})

	session := awsprovider.NewSession()
	err := manager.RegisterWebhooks().RegisterControllers(
		infra.NewVPCController(awsprovider.EC2Client(session)),
		infra.NewSubnetController(awsprovider.EC2Client(session)),
		infra.NewInternetGWController(awsprovider.EC2Client(session)),
	).Start(controllerruntime.SetupSignalHandler())
	log.PanicIfError(err, "Unable to start manager")
}
