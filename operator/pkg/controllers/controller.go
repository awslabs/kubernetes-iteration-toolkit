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

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/status"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
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
	var err error
	if err = c.Get(ctx, req.NamespacedName, resource); err != nil {
		if errors.KubeObjNotFound(err) {
			return reconcile.Result{}, nil
		}
		return *status.Failed, err
	}
	// 2. Copy object for merge patch base
	persisted := resource.DeepCopyObject()
	// 3. Reconcile else finalize if object is deleted
	result, reconcileErr := c.reconcile(ctx, resource, persisted)
	// 4. Update Status using a merge patch, we want to set status even when reconcile errored
	if err := c.Status().Patch(ctx, resource, client.MergeFrom(persisted)); err != nil && !errors.KubeObjNotFound(err) {
		return *status.Failed, fmt.Errorf("status patch for %s, %w,", req.NamespacedName, err)
	}
	if reconcileErr != nil {
		zap.S().Errorf("Error reconciling the resource %v", err)
		return *status.Failed, reconcileErr
	}
	return result, nil
}

func (c *GenericController) reconcile(ctx context.Context, resource Object, persisted runtime.Object) (reconcile.Result, error) {
	var result *reconcile.Result
	var err error
	existingFinalizers := resource.GetFinalizers()
	if resource.GetDeletionTimestamp() == nil {
		// Add finalizer for this controller if not exists
		c.addFinalizer(ctx, resource)
		result, err = c.Controller.Reconcile(ctx, resource)
		if err != nil {
			resource.StatusConditions().MarkFalse(v1alpha1.Active, "", err.Error())
			zap.S().Errorf("Failed to reconcile kind %s, %v", resource.GetObjectKind().GroupVersionKind().Kind, err)
			return *status.Failed, fmt.Errorf("reconciling resource, %w", err)
		}
		resource.StatusConditions().MarkTrue(v1alpha1.Active)
	} else {
		if result, err = c.Controller.Finalize(ctx, resource); err != nil {
			return *status.Failed, fmt.Errorf("finalizing resource controller %v, %w", c.Controller.Name(), err)
		}
		c.removeFinalizer(ctx, resource)
		zap.S().Infof("[%s] Successfully deleted", resource.GetName())
	}
	// If the finalizers have changed merge patch the object
	if len(existingFinalizers) != len(resource.GetFinalizers()) {
		if err := c.Patch(ctx, resource, client.MergeFrom(persisted)); err != nil {
			return *status.Failed, fmt.Errorf("patch object %s, %w", resource.GetName(), err)
		}
	}
	return *result, nil
}

func (c *GenericController) addFinalizer(ctx context.Context, resource Object) {
	finalizerStr := fmt.Sprintf(FinalizerForAWSResources, c.Name())
	for _, finalizer := range resource.GetFinalizers() {
		if finalizer == finalizerStr {
			return
		}
	}
	finalizers := append(resource.GetFinalizers(), finalizerStr)
	resource.SetFinalizers(finalizers)
}

func (c *GenericController) removeFinalizer(ctx context.Context, resource Object) {
	finalizerStr := fmt.Sprintf(FinalizerForAWSResources, c.Name())
	remainingFinalizers := []string{}
	for _, finalizer := range resource.GetFinalizers() {
		if finalizer == finalizerStr {
			continue
		}
		remainingFinalizers = append(remainingFinalizers, finalizer)
	}
	if len(remainingFinalizers) < len(resource.GetFinalizers()) {
		resource.SetFinalizers(remainingFinalizers)
	}
}
