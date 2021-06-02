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
	"fmt"

	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/errors"
	"github.com/prateekgogia/kit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	FinalizerForAWSResources = "kit.k8s.amazonaws.com/%s"
)

// GenericController implements controllerruntime.Reconciler and runs a
// standardized reconciliation workflow against incoming resource watch events.
type GenericController struct {
	Controller
	client.Client
}

// Reconcile executes a control loop for the resource
func (c *GenericController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	// 1. Read Spec
	resource := c.For()
	if err := c.Get(ctx, req.NamespacedName, resource); err != nil {
		if errors.KubeObjNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	// 2. Copy object for merge patch base
	persisted := resource.DeepCopyObject()
	// 3. reconcile else finalize if object is deleted
	result, err := c.reconcile(ctx, resource)
	if err != nil {
		resource.StatusConditions().MarkFalse(v1alpha1.Active, "", err.Error())
		// if errors.SafeToIgnore(err) {
		// 	return reconcile.Result{}, err
		// }
		zap.S().Errorf("Failed to reconcile kind %s, %v", resource.GetObjectKind().GroupVersionKind().Kind, err)
		return reconcile.Result{}, err
	}
	resource.StatusConditions().MarkTrue(v1alpha1.Active)
	// 4. Update Status using a merge patch
	if err := c.Status().Patch(ctx, resource, client.MergeFrom(persisted)); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to persist changes to %s, %w", req.NamespacedName, err)
	}
	return *result, nil
}

func (c *GenericController) reconcile(ctx context.Context, resource Object) (*reconcile.Result, error) {
	if resource.GetDeletionTimestamp() == nil {
		// Add finalizer for this controller if not exists
		if err := c.addFinalizerIfNotExists(ctx, resource); err != nil {
			return nil, err
		}
		result, err := c.Controller.Reconcile(ctx, resource)
		if err != nil {
			return nil, fmt.Errorf("reconciling resource, %w", err)
		}
		return result, nil
	}
	zap.S().Infof("FInalizing for resource %v controller %v", resource.GetName(), c.Name())
	result, err := c.Controller.Finalize(ctx, resource)
	if err != nil {
		zap.S().Errorf("Error while finalizing resource %v controller %v %v", resource.GetName(), c.Name(), err)
		return nil, fmt.Errorf("finalizing resource controller name %v, %w", c.Controller.Name(), err)
	}
	zap.S().Infof("Removing finalizer for resource %v controller %v", resource.GetName(), c.Name())
	if err := c.removeFinalizer(ctx, resource); err != nil {
		return status.Waiting, fmt.Errorf("removing finalizers, %w", err)
	}
	return result, nil
}

func (c *GenericController) addFinalizerIfNotExists(ctx context.Context, resource Object) error {
	if c.finalizerExists(resource) {
		return nil
	}
	finalizerStr := fmt.Sprintf(FinalizerForAWSResources, c.Name())
	finalizers := append(resource.GetFinalizers(), finalizerStr)
	if err := c.patchFinalizersToResource(ctx, resource, finalizers); err != nil {
		return err
	}
	return nil
}

func (c *GenericController) finalizerExists(resource Object) bool {
	finalizerStr := fmt.Sprintf(FinalizerForAWSResources, c.Name())
	for _, finalizer := range resource.GetFinalizers() {
		if finalizer == finalizerStr {
			return true
		}
	}
	return false
}

func (c *GenericController) removeFinalizer(ctx context.Context, resource Object) error {
	finalizerStr := fmt.Sprintf(FinalizerForAWSResources, c.Name())
	remainingFinalizers := []string{}
	for _, finalizer := range resource.GetFinalizers() {
		if finalizer == finalizerStr {
			continue
		}
		remainingFinalizers = append(remainingFinalizers, finalizer)
	}
	if len(remainingFinalizers) < len(resource.GetFinalizers()) {
		if err := c.patchFinalizersToResource(ctx, resource, remainingFinalizers); err != nil {
			return err
		}
		zap.S().Infof("Successfully deleted finalizer %s for cluster name %s", finalizerStr, resource.GetName())
	}
	return nil
}

func (c *GenericController) patchFinalizersToResource(ctx context.Context, resource Object, finalizers []string) error {
	persisted := resource.DeepCopyObject()
	resource.SetFinalizers(finalizers)
	if err := c.Patch(ctx, resource, client.MergeFrom(persisted)); err != nil {
		return fmt.Errorf("merging changes to kube object, %w", err)
	}
	return nil
}
