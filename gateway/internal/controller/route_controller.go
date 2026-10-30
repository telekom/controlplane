// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/types"
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
		For(&gatewayv1.Route{}, builder.WithPredicates(cc.Count("route", cc.RoleFor,
			predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{})))).
		Watches(&gatewayv1.ConsumeRoute{},
			handler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToRoute),
			builder.WithPredicates(cc.Count("route", cc.RoleWatches, predicate.GenerationChangedPredicate{}, cc.SkipInitialListPredicate{}))).
		Watches(&gatewayv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.mapGatewayToRoutes),
			builder.WithPredicates(cc.Count("route", cc.RoleWatches, NamePrefixChangedPredicate()))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
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

// NamePrefixChangedPredicate passes only updates of a Gateway that change spec.namePrefix.
func NamePrefixChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldGateway, okOld := e.ObjectOld.(*gatewayv1.Gateway)
			newGateway, okNew := e.ObjectNew.(*gatewayv1.Gateway)
			return okOld && okNew && oldGateway.Spec.NamePrefix != newGateway.Spec.NamePrefix
		},
	}
}

// mapGatewayToRoutes enqueues every Route that references the Gateway, sorted by key.
func (r *RouteReconciler) mapGatewayToRoutes(ctx context.Context, obj client.Object) []reconcile.Request {
	gateway, ok := obj.(*gatewayv1.Gateway)
	if !ok {
		return nil
	}

	routes := &gatewayv1.RouteList{}
	err := r.List(ctx, routes, client.MatchingFields{IndexFieldSpecGatewayRef: types.ObjectRefFromObject(gateway).String()})
	if err != nil {
		ctrllog.FromContext(ctx).Error(err, "Failed to list routes of gateway", "gateway", gateway.Name)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(routes.Items))
	for i := range routes.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&routes.Items[i])})
	}
	slices.SortFunc(requests, func(a, b reconcile.Request) int { return strings.Compare(a.String(), b.String()) })
	return requests
}
