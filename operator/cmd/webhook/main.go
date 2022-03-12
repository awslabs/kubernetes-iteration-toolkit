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
	"flag"

	cpv1alpha1 "github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	dpv1alpha1 "github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/dataplane/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/signals"
	"knative.dev/pkg/system"
	"knative.dev/pkg/webhook"
	"knative.dev/pkg/webhook/certificates"
	"knative.dev/pkg/webhook/resourcesemantics"
	"knative.dev/pkg/webhook/resourcesemantics/defaulting"
	"knative.dev/pkg/webhook/resourcesemantics/validation"
)

var (
	options      = Options{}
	kitResources = map[schema.GroupVersionKind]resourcesemantics.GenericCRD{}
)

type Options struct {
	Port int
}

func main() {
	flag.IntVar(&options.Port, "port", 8443, "The port the webhook endpoint binds to for validation and mutation of resources")
	flag.Parse()

	config := injection.ParseAndGetRESTConfigOrDie()

	// merge all kit resource handlers
	kitResources = cpv1alpha1.Resources
	for gvk, resource := range dpv1alpha1.Resources {
		kitResources[gvk] = resource
	}

	// Controllers and webhook
	sharedmain.MainWithConfig(
		webhook.WithOptions(injection.WithNamespaceScope(signals.NewContext(), system.Namespace()), webhook.Options{
			Port:        options.Port,
			ServiceName: "kit-webhook",
			SecretName:  "kit-webhook-cert",
		}),
		"kit.webhooks",
		config,
		certificates.NewController,
		NewCRDDefaultingWebhook,
		NewCRDValidationWebhook,
	)
}

func NewCRDDefaultingWebhook(ctx context.Context, w configmap.Watcher) *controller.Impl {
	return defaulting.NewAdmissionController(ctx,
		"defaulting.webhook.kit.k8s.sh",
		"/default-resource",
		kitResources,
		InjectContext,
		true,
	)
}

func NewCRDValidationWebhook(ctx context.Context, w configmap.Watcher) *controller.Impl {
	return validation.NewAdmissionController(ctx,
		"validation.webhook.kit.k8s.sh",
		"/validate-resource",
		kitResources,
		InjectContext,
		true,
	)
}

func InjectContext(ctx context.Context) context.Context { return ctx }
