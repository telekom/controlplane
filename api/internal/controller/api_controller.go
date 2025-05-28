// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/api"
	"github.com/telekom/controlplane/common/pkg/config"
	ccontroller "github.com/telekom/controlplane/common/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ApiReconciler reconciles a Api object
type ApiReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	ccontroller.Controller[*apiapi.Api]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apis/finalizers,verbs=update

func (r *ApiReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &apiapi.Api{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApiReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("api-controller")
	r.Controller = ccontroller.NewController(&api.ApiHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiapi.Api{}).
		Watches(&apiapi.Api{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiToApi),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
			RateLimiter:             workqueue.DefaultTypedItemBasedRateLimiter[reconcile.Request](),
		}).
		Complete(r)
}

func (r *ApiReconciler) MapApiToApi(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	api, ok := obj.(*apiapi.Api)
	if !ok {
		log.Info("object is not an API")
		return nil
	}

	list := &apiapi.ApiList{}
	err := r.Client.List(ctx, list, client.MatchingLabels{
		config.EnvironmentLabelKey: api.Labels[config.EnvironmentLabelKey],
		apiapi.BasePathLabelKey:    api.Labels[apiapi.BasePathLabelKey],
	})
	if err != nil {
		log.Error(err, "failed to list API-Subscriptions")
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		if api.UID == item.UID {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
	}

	return reqs
}
