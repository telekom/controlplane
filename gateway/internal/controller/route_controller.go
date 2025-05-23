// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	routehandler "github.com/telekom/controlplane/gateway/internal/handler/route"
)

// RouteReconciler reconciles a Route object
type RouteReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*gatewayv1.Route]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes/finalizers,verbs=update

func (r *RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &gatewayv1.Route{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("route-controller")
	r.Controller = cc.NewController(&routehandler.RouteHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Route{}).
		Watches(&gatewayv1.Realm{},
			handler.EnqueueRequestsFromMapFunc(r.mapRealmToRoute),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(&gatewayv1.ConsumeRoute{},
			handler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToRoute),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
			RateLimiter:             workqueue.DefaultTypedItemBasedRateLimiter[reconcile.Request](),
		}).
		Complete(r)
}

func (r *RouteReconciler) mapConsumeRouteToRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	// ensure its actually a ConsumeRoute
	consumeRoute, ok := obj.(*gatewayv1.ConsumeRoute)
	if !ok {
		return nil
	}

	// get the Route
	route := &gatewayv1.Route{}
	if err := r.Get(ctx, consumeRoute.Spec.Route.K8s(), route); err != nil {
		return nil
	}

	return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: route.Name, Namespace: route.Namespace}}}
}

func (r *RouteReconciler) mapRealmToRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	// ensure its actually a Realm
	realm, ok := obj.(*gatewayv1.Realm)
	if !ok {
		return nil
	}
	if realm.Labels == nil {
		return nil
	}

	listOpts := []client.ListOption{
		client.MatchingFields{
			IndexFieldSpecRealm: types.ObjectRefFromObject(realm).String(),
		},
		client.MatchingLabels{
			config.EnvironmentLabelKey: realm.Labels[config.EnvironmentLabelKey],
		},
	}

	list := gatewayv1.RouteList{}
	if err := r.List(ctx, &list, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(list.Items))
	for i, item := range list.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKey{Name: item.Name, Namespace: item.Namespace}}
	}

	return requests
}
