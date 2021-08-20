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
	"reflect"

	"github.com/awslabs/kit/operator/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/results"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
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
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return *results.Failed, err
	}
	if resource.GetObjectKind().GroupVersionKind().Empty() {
		resource.GetObjectKind().SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ControlPlaneKind))
	}
	// 2. Copy object for merge patch base
	persisted := resource.DeepCopyObject()
	// 3. Reconcile else finalize if object is deleted
	result, reconcileErr := c.reconcile(ctx, resource, persisted)
	// 4. Update Status using a merge patch, we want to set status even when reconcile errored
	if err := c.Status().Patch(ctx, resource, client.MergeFrom(persisted)); err != nil && !errors.IsNotFound(err) {
		return *results.Failed, fmt.Errorf("status patch for %s, %w,", req.NamespacedName, err)
	}
	if reconcileErr != nil {
		if errors.IsWaitingForSubResource(reconcileErr) {
			return *results.Waiting, nil
		}
		return *results.Failed, reconcileErr
	}
	return result, nil
}

func (c *GenericController) reconcile(ctx context.Context, resource Object, persisted runtime.Object) (reconcile.Result, error) {
	var result *reconcile.Result
	var err error
	existingFinalizers := resource.GetFinalizers()
	existingFinalizerSet := sets.NewString(existingFinalizers...)
	finalizerStr := sets.NewString(fmt.Sprintf(FinalizerForAWSResources, c.Name()))
	if resource.GetDeletionTimestamp() == nil {
		// Add finalizer for this controller
		resource.SetFinalizers(existingFinalizerSet.Union(finalizerStr).UnsortedList())
		result, err = c.Controller.Reconcile(ctx, resource)
		if err != nil {
			resource.StatusConditions().MarkFalse(v1alpha1.Active, "", err.Error())
			return *results.Failed, fmt.Errorf("reconciling resource, %w", err)
		}
		resource.StatusConditions().MarkTrue(v1alpha1.Active)
	} else {
		if result, err = c.Controller.Finalize(ctx, resource); err != nil {
			return *results.Failed, fmt.Errorf("finalizing resource controller %v, %w", c.Controller.Name(), err)
		}
		// Remove finalizer for this controller
		resource.SetFinalizers(existingFinalizerSet.Difference(finalizerStr).UnsortedList())
		zap.S().Infof("[%s] Successfully deleted", resource.GetName())
	}
	// If the finalizers have changed merge patch the object
	if !reflect.DeepEqual(existingFinalizers, resource.GetFinalizers()) {
		if err := c.Patch(ctx, resource, client.MergeFrom(persisted)); err != nil {
			return *results.Failed, fmt.Errorf("patch object %s, %w", resource.GetName(), err)
		}
	}
	return *result, nil
}
