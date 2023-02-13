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
	"time"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/results"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	FinalizerForAWSResources = v1alpha1.SchemeGroupVersion.Group + "/%s"
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
	/*
		Need to set this for tests to pass, in testing the client.Client used
		doesn't populate GVK due at a bug in client-go. We can remove this if check,
		once this bug is fixed https://github.com/kubernetes/client-go/issues/1004
	*/
	if resource.GetObjectKind().GroupVersionKind().Empty() {
		resource.GetObjectKind().SetGroupVersionKind(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ControlPlaneKind))
	}
	// 2. Copy object for merge patch base
	persisted := resource.DeepCopyObject()
	// 3. Reconcile else finalize if object is deleted
	result, reconcileErr := c.reconcile(ctx, resource, persisted)
	// 4. Update Status using a merge patch, we want to set status even when reconcile errored
	if err := c.Status().Patch(ctx, resource, client.MergeFrom(persisted.(client.Object))); err != nil && !errors.IsNotFound(err) {
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
	if resource.GetDeletionTimestamp() == nil && ttlExpiredForControlPlane(resource) {
		// when ttl expires for the cluster, finalize all sub resources
		if result, err = c.finalizeSubResources(ctx, resource, existingFinalizerSet.Difference(finalizerStr).UnsortedList()); err != nil {
			return *results.Failed, fmt.Errorf("finalizing resource controller %w", err)
		}
		if result.Requeue || result.RequeueAfter > 0 {
			return *result, nil
		}
		// delete the CP object since it doesn't get auto deleted.
		if err := c.Delete(ctx, resource); err != nil {
			return *results.Failed, fmt.Errorf("deleting CP object, %w", err)
		}
		// once the object is deleted we don't need to merge patch anymore
		return reconcile.Result{}, nil
	}
	if resource.GetDeletionTimestamp() != nil {
		if result, err = c.finalizeSubResources(ctx, resource, existingFinalizerSet.Difference(finalizerStr).UnsortedList()); err != nil {
			return *results.Failed, fmt.Errorf("finalizing resource controller %v, %w", c.Controller.Name(), err)
		}
	} else {
		// Add finalizer for this controller
		resource.SetFinalizers(existingFinalizerSet.Union(finalizerStr).UnsortedList())
		result, err = c.Controller.Reconcile(ctx, resource)
		if err != nil {
			resource.StatusConditions().MarkFalse(v1alpha1.Active, "", err.Error())
			return *results.Failed, fmt.Errorf("reconciling resource, %w", err)
		}
		resource.StatusConditions().MarkTrue(v1alpha1.Active)
	}
	// If the finalizers have changed merge patch the object
	if !reflect.DeepEqual(existingFinalizers, resource.GetFinalizers()) {
		if err := c.Patch(ctx, resource, client.MergeFrom(persisted.(client.Object))); err != nil {
			return *results.Failed, fmt.Errorf("patch object %s, %w", resource.GetName(), err)
		}
	}
	return *result, nil
}

func (c *GenericController) finalizeSubResources(ctx context.Context, resource Object, finalizers []string) (*reconcile.Result, error) {
	result, err := c.Controller.Finalize(ctx, resource)
	if err != nil {
		return result, fmt.Errorf("finalizing resource controller %v, %w", c.Controller.Name(), err)
	}
	// Remove finalizer for this controller
	resource.SetFinalizers(finalizers)
	zap.S().Infof("[%s] Successfully deleted resources", resource.GetName())
	return result, nil
}

func ttlExpiredForControlPlane(resource Object) bool {
	cp, ok := resource.(*v1alpha1.ControlPlane)
	if !ok {
		return false
	}
	if cp.Spec.TTL != "" {
		duration, err := time.ParseDuration(cp.Spec.TTL)
		if err != nil {
			zap.S().Errorf("parsing TTL duration, %w", err)
			return false
		}
		deleteAfter := resource.GetCreationTimestamp().Add(duration)
		if time.Now().After(deleteAfter) {
			zap.S().Infof("[%v] control plane TTL expired, deleting cluster resources", cp.ClusterName())
			return true
		}
	}
	return false
}
