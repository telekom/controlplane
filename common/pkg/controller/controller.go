// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
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

	common_types "github.com/telekom/controlplane/common/pkg/types"

	cc "github.com/telekom/controlplane/common/pkg/client"
)

type Controller[T common_types.Object] interface {
	Reconcile(context.Context, reconcile.Request, T) (reconcile.Result, error)
}

var _ Controller[common_types.Object] = &ControllerImpl[common_types.Object]{}

func NewController[T common_types.Object](handler handler.Handler[T], client client.Client, recorder record.EventRecorder) Controller[T] {
	return &ControllerImpl[T]{
		Client: client,
		Scheme: client.Scheme(),

		Recorder: recorder,
		Handler:  handler,
	}
}

type ControllerImpl[T common_types.Object] struct {
	Client client.Client
	Scheme *runtime.Scheme

	Handler  handler.Handler[T]
	Recorder record.EventRecorder
}

func (c *ControllerImpl[T]) Reconcile(ctx context.Context, req reconcile.Request, object T) (reconcile.Result, error) {
	log := log.FromContext(ctx)

	err := Fetch(ctx, c.Client, req.NamespacedName, object)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Fetched object but it was not found")
			return reconcile.Result{}, nil
		}
		return HandleError(ctx, err, object, c.Recorder), nil
	}

	if changed, err := FirstSetup(ctx, c.Client, object); err != nil {
		return HandleError(ctx, err, object, c.Recorder), nil
	} else if changed {
		return reconcile.Result{}, nil
	}

	log.V(1).Info("Fetched object")

	env, ok := GetEnvironment(object)
	if !ok {
		log.V(0).Info("Environment label is missing")
		c.Event(ctx, object, "Warning", "Processing", "Environment label is missing")
		if object.SetCondition(condition.NewBlockedCondition("Environment label is missing")) {
			if err := c.Client.Status().Update(ctx, object); err != nil {
				return HandleError(ctx, err, object, c.Recorder), nil
			}
		}
		return reconcile.Result{}, nil
	}

	ctx = contextutil.WithEnv(ctx, env)
	ctx = cc.WithClient(ctx, cc.NewJanitorClient(cc.NewScopedClient(c.Client, env)))
	ctx = contextutil.WithRecorder(ctx, c.Recorder)

	// Handle the deletion
	if IsBeingDeleted(object) {
		c.Event(ctx, object, "Normal", "Processing", "Processing resource deletion")
		if controllerutil.ContainsFinalizer(object, config.FinalizerName) {
			log.V(0).Info("Deleting")

			err := c.Handler.Delete(ctx, object)
			if err != nil {
				// Failed
				EnsureNotReadyOnError(ctx, c.Client, object, err)
				result := HandleError(ctx, err, object, c.Recorder)
				if err = c.Client.Status().Update(ctx, object); err != nil {
					return HandleError(ctx, err, object, c.Recorder), nil
				}
				return result, nil
			}

			// Success

			changed := controllerutil.RemoveFinalizer(object, config.FinalizerName)
			if changed {
				err = c.Client.Update(ctx, object)
				if err != nil {
					return HandleError(ctx, err, object, c.Recorder), nil
				}
			}

			log.V(1).Info("Deleted", "resource", object)
		}
		return reconcile.Result{}, nil
	}

	// Handle normal reconciliation
	c.Event(ctx, object, "Normal", "Processing", "Processing resource")

	log.V(1).Info("Creating or updating")
	err = c.Handler.CreateOrUpdate(ctx, object)
	if err != nil {
		// Failed
		EnsureNotReadyOnError(ctx, c.Client, object, err)
		result := HandleError(ctx, err, object, c.Recorder)
		// Always update the status after reconciliation to persist any changes made by the handler
		// and to set any conditions related to the error.
		if err = c.Client.Status().Update(ctx, object); err != nil {
			return HandleError(ctx, err, object, c.Recorder), nil
		}
		return result, nil
	}

	// Success

	log.V(1).Info("Created or updated", "resource", object)
	// Enforce that atleast the processing condition is set in the handler. If not, log a warning.
	if meta.IsStatusConditionPresentAndEqual(object.GetConditions(), condition.ConditionTypeProcessing, metav1.ConditionUnknown) {
		c.Event(ctx, object, "Warning", "Processing", "Resource has an unknown processing status")
	}

	// Always update the status after successful reconciliation to clear any error conditions
	// and persist any changes made by the handler.
	if err = c.Client.Status().Update(ctx, object); err != nil {
		return HandleError(ctx, err, object, c.Recorder), nil
	}

	return reconcile.Result{
		RequeueAfter: config.RequeueWithJitter(),
	}, nil
}

func (c *ControllerImpl[T]) Event(ctx context.Context, object common_types.Object, eventType, reason, message string) {
	if c.Recorder != nil {
		c.Recorder.Event(object, eventType, reason, message)
	}
}

func Fetch(ctx context.Context, client client.Client, namespacedName types.NamespacedName, object client.Object) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Fetching object")

	if err := client.Get(ctx, namespacedName, object); err != nil {
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

func FirstSetup(ctx context.Context, client client.Client, object common_types.Object) (bool, error) {
	if !controllerutil.ContainsFinalizer(object, config.FinalizerName) {
		controllerutil.AddFinalizer(object, config.FinalizerName)
		if err := client.Update(ctx, object); err != nil {
			return false, err
		}
		return true, nil
	}

	// According to the best-practice:
	// "Controllers should apply their conditions to a resource the first time they visit the resource, even if the status is Unknown"
	// see https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	if len(object.GetConditions()) == 0 {
		object.SetCondition(condition.SetToUnknown(condition.ReadyCondition))
		object.SetCondition(condition.SetToUnknown(condition.ProcessingCondition))
	}

	return false, nil
}

func HandleError(ctx context.Context, err error, obj common_types.Object, recorder record.EventRecorder) reconcile.Result {
	// handle Conflict - resource version can change during reconciliation, it causes conflict, simple requeue should solve it
	if apierrors.IsConflict(err) {
		log := log.FromContext(ctx).WithName("controller.error-handler")
		log.V(0).Info("Conflict occurred during operation", "error", err)
		if recorder != nil {
			recorder.Event(obj, "Warning", "Conflict", err.Error())
		}
		return reconcile.Result{RequeueAfter: config.RetryWithJitterOnError()}
	}

	_, result := ctrlerrors.HandleError(ctx, obj, err, recorder)
	return result
}

// EnsureNotReadyOnError sets the Ready condition to false on the object if the error is not nil
// and the Ready condition is not already set to false.
func EnsureNotReadyOnError(ctx context.Context, client client.Client, obj common_types.Object, err error) bool {
	if err != nil && !meta.IsStatusConditionFalse(obj.GetConditions(), condition.ConditionTypeReady) {
		return obj.SetCondition(condition.NewNotReadyCondition("ErrorOccurred", err.Error()))
	}
	return false
}
