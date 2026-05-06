// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	common_types "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

type Controller[T common_types.Object] interface {
	Reconcile(context.Context, reconcile.Request, T) (reconcile.Result, error)
}

var _ Controller[common_types.Object] = &ControllerImpl[common_types.Object]{}

func NewController[T common_types.Object](h handler.Handler[T], c client.Client, recorder record.EventRecorder) Controller[T] {
	return &ControllerImpl[T]{
		Client: c,
		Scheme: c.Scheme(),

		Recorder: recorder,
		Handler:  h,
	}
}

type ControllerImpl[T common_types.Object] struct {
	Client client.Client
	Scheme *runtime.Scheme

	Handler  handler.Handler[T]
	Recorder record.EventRecorder
}

func (c *ControllerImpl[T]) Reconcile(ctx context.Context, req reconcile.Request, object T) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	err := Fetch(ctx, c.Client, req.NamespacedName, object)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Fetched object but it was not found")
			return reconcile.Result{}, nil
		}
		return HandleError(ctx, err, object, c.Recorder), nil
	}

	if changed, setupErr := FirstSetup(ctx, c.Client, object); setupErr != nil {
		return HandleError(ctx, setupErr, object, c.Recorder), nil
	} else if changed {
		return reconcile.Result{}, nil
	}

	logger.V(1).Info("Fetched object")

	env, ok := GetEnvironment(object)
	if !ok {
		logger.V(0).Info("Environment label is missing")
		c.Event(ctx, object, "Warning", "Processing", "Environment label is missing")
		if object.SetCondition(condition.NewBlockedCondition("Environment label is missing")) {
			StampObservedGeneration(object)
			if updateErr := c.Client.Status().Update(ctx, object); updateErr != nil {
				return HandleError(ctx, updateErr, object, c.Recorder), nil
			}
		}
		return reconcile.Result{}, nil
	}

	ctx = contextutil.WithEnv(ctx, env)
	ctx = cc.WithClient(ctx, cc.NewJanitorClient(cc.NewScopedClient(c.Client, env)))
	ctx = contextutil.WithRecorder(ctx, c.Recorder)

	hint := &contextutil.ReconcileHint{}
	ctx = contextutil.WithReconcileHint(ctx, hint)

	// Handle the deletion
	if IsBeingDeleted(object) {
		return c.handleDeletion(ctx, object)
	}

	c.Event(ctx, object, "Normal", "Processing", "Processing resource")

	logger.V(1).Info("Creating or updating")
	err = c.Handler.CreateOrUpdate(ctx, object)
	if err != nil {
		EnsureNotReadyOnError(ctx, c.Client, object, err)
		result := HandleError(ctx, err, object, c.Recorder)
		StampObservedGeneration(object)
		if statusErr := c.Client.Status().Update(ctx, object); statusErr != nil {
			return HandleError(ctx, statusErr, object, c.Recorder), nil
		}
		return result, nil
	}

	// Success

	logger.V(1).Info("Created or updated", "resource", object)
	// Enforce that at least the ready condition is set in the handler. If not, log a warning.
	if meta.IsStatusConditionPresentAndEqual(object.GetConditions(), condition.ConditionTypeReady, metav1.ConditionUnknown) {
		c.Event(ctx, object, "Warning", "UnknownReady", "Resource has an unknown ready status")
	}

	StampObservedGeneration(object)
	if err = c.Client.Status().Update(ctx, object); err != nil {
		return HandleError(ctx, err, object, c.Recorder), nil
	}

	requeueAfter := config.RequeueWithJitter()
	if hint.RequeueAfter != nil && *hint.RequeueAfter < requeueAfter {
		requeueAfter = *hint.RequeueAfter
	}

	return reconcile.Result{
		RequeueAfter: requeueAfter,
	}, nil
}

func (c *ControllerImpl[T]) handleDeletion(ctx context.Context, object T) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	c.Event(ctx, object, "Normal", "Processing", "Processing resource deletion")

	if !controllerutil.ContainsFinalizer(object, config.FinalizerName) {
		return reconcile.Result{}, nil
	}

	logger.V(0).Info("Deleting")
	err := c.Handler.Delete(ctx, object)
	if err != nil {
		EnsureNotReadyOnError(ctx, c.Client, object, err)
		result := HandleError(ctx, err, object, c.Recorder)
		StampObservedGeneration(object)
		if statusErr := c.Client.Status().Update(ctx, object); statusErr != nil {
			return HandleError(ctx, statusErr, object, c.Recorder), nil
		}
		return result, nil
	}

	if controllerutil.RemoveFinalizer(object, config.FinalizerName) {
		if updateErr := c.Client.Update(ctx, object); updateErr != nil {
			return HandleError(ctx, updateErr, object, c.Recorder), nil
		}
	}

	logger.V(1).Info("Deleted", "resource", object)
	return reconcile.Result{}, nil
}

func (c *ControllerImpl[T]) Event(ctx context.Context, object common_types.Object, eventType, reason, message string) {
	if c.Recorder != nil {
		c.Recorder.Event(object, eventType, reason, message)
	}
}

func Fetch(ctx context.Context, c client.Client, namespacedName types.NamespacedName, object client.Object) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Fetching object")

	if err := c.Get(ctx, namespacedName, object); err != nil {
		return err
	}
	return nil
}

func IsBeingDeleted(object metav1.Object) bool {
	return !object.GetDeletionTimestamp().IsZero()
}

func GetEnvironment(object metav1.Object) (string, bool) {
	labels := object.GetLabels()
	if labels == nil {
		return "", false
	}
	e, ok := labels[config.EnvironmentLabelKey]
	return e, ok
}

func FirstSetup(ctx context.Context, c client.Client, object common_types.Object) (bool, error) {
	if !controllerutil.ContainsFinalizer(object, config.FinalizerName) {
		controllerutil.AddFinalizer(object, config.FinalizerName)
		if err := c.Update(ctx, object); err != nil {
			return false, err
		}
		return true, nil
	}

	// According to the best-practice:
	// "Controllers should apply their conditions to a resource the first time they visit the resource, even if the status is Unknown"
	// see https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	if len(object.GetConditions()) == 0 {
		object.SetCondition(condition.SetToUnknown(condition.ReadyCondition))
	}

	return false, nil
}

func HandleError(ctx context.Context, err error, obj common_types.Object, recorder record.EventRecorder) reconcile.Result {
	if apierrors.IsConflict(err) {
		logger := log.FromContext(ctx).WithName("controller.error-handler")
		logger.V(0).Info("Conflict occurred during operation", "error", err)
		if recorder != nil {
			recorder.Event(obj, "Warning", "Conflict", err.Error())
		}
		return reconcile.Result{RequeueAfter: config.RetryWithJitterOnError()}
	}

	conditionsUpdated, result := ctrlerrors.HandleError(ctx, obj, err, recorder)
	if conditionsUpdated {
		logger := log.FromContext(ctx).WithName("controller.error-handler")
		logger.V(1).Info("Object conditions updated after error handling", "error", err)
	}
	return result
}

// EnsureNotReadyOnError sets the Ready condition to false on the object if the error is not nil
// and the Ready condition is not already set to false.
func EnsureNotReadyOnError(_ context.Context, _ client.Client, obj common_types.Object, err error) bool {
	if err != nil && !meta.IsStatusConditionFalse(obj.GetConditions(), condition.ConditionTypeReady) {
		return obj.SetCondition(condition.NewNotReadyCondition("ErrorOccurred", err.Error()))
	}
	return false
}

// StampObservedGeneration sets ObservedGeneration on all conditions to match
// the object's current metadata.generation, per Kubernetes API conventions.
// This must be called immediately before Status().Update() to ensure consumers
// can detect stale conditions (where the spec changed but the controller has
// not yet reconciled).
func StampObservedGeneration(obj common_types.Object) {
	conditions := obj.GetConditions()
	gen := obj.GetGeneration()
	for i := range conditions {
		conditions[i].ObservedGeneration = gen
	}
}
