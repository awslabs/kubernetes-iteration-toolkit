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
	"context"

	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Controller is an interface implemented by AWS resources like VPC, Subnet,
// Security groups etc. required for a cluster creation
type Controller interface {
	// Name returns the name of controller for the resource (vpc, subnet) to
	// identify which controller is running and to add info to the logs.
	Name() string
	// Reconcile hands a hydrated kubernetes resource to the controller for
	// reconciliation. Any changes made to the resource's status are persisted
	// after Reconcile returns, even if it returns an error.
	Reconcile(context.Context, Object) (*reconcile.Result, error)
	// Reconcile hands a hydrated kubernetes resource to the controller for
	// cleanup. Any changes made to the resource's status are persisted after
	// Finalize returns, even if it returns an error.
	Finalize(context.Context, Object) (*reconcile.Result, error)
	// For returns a default instantiation of the resource and is injected by
	// data from the API Server at the start of the reconciliation loop.
	For() Object
	// DeepCopy returns a copy of the object provided
	DeepCopy(Object) Object
}

// Webhook implements both a handler and path and can be attached to a webhook server.
type Webhook interface {
	webhook.AdmissionHandler
	Path() string
}

// Object provides an abstraction over a kubernetes custom resource with
// methods necessary to standardize reconciliation behavior in kit.
type Object interface {
	client.Object
	StatusConditions() apis.ConditionManager
}

// Manager manages a set of controllers and webhooks.
type Manager interface {
	manager.Manager
	RegisterControllers(controllers ...Controller) Manager
	RegisterWebhooks(controllers ...Webhook) Manager
}
