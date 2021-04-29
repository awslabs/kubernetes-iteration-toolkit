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

package controllers

import (
	"time"

	"github.com/awslabs/karpenter/pkg/utils/log"

	"golang.org/x/time/rate"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type GenericControllerManager struct {
	manager.Manager
}

// NewManagerOrDie instantiates a controller manager or panics
func NewManagerOrDie(config *rest.Config, options controllerruntime.Options) Manager {
	manager, err := controllerruntime.NewManager(config, options)
	log.PanicIfError(err, "Failed to create controller manager")
	return &GenericControllerManager{Manager: manager}
}

// RegisterControllers registers a set of controllers to the controller manager
func (m *GenericControllerManager) RegisterControllers(controllers ...Controller) Manager {
	for _, c := range controllers {
		controlledObject := c.For()
		builder := controllerruntime.NewControllerManagedBy(m).For(controlledObject).WithOptions(controller.Options{
			RateLimiter: workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(100*time.Millisecond, 10*time.Second),
				// 10 qps, 100 bucket size
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
			),
		})
		if namedController, ok := c.(NamedController); ok {
			builder.Named(namedController.Name())
		}
		for _, resource := range c.Owns() {
			builder = builder.Owns(resource)
		}
		log.PanicIfError(builder.Complete(&GenericController{Controller: c, Client: m.GetClient()}),
			"Failed to register controller to manager for %s", controlledObject)
		log.PanicIfError(controllerruntime.NewWebhookManagedBy(m).For(controlledObject).Complete(),
			"Failed to register controller to manager for %s", controlledObject)
	}
	return m
}

// RegisterWebhooks registers a set of webhooks to the controller manager
func (m *GenericControllerManager) RegisterWebhooks(webhooks ...Webhook) Manager {
	for _, w := range webhooks {
		m.GetWebhookServer().Register(w.Path(), &webhook.Admission{Handler: w})
	}
	return m
}
