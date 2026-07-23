// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
		For(&gatewayv1.Route{}).
		Watches(&gatewayv1.ConsumeRoute{},
			handler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToRoute),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&gatewayv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.mapGatewayToRoutes),
			builder.WithPredicates(predicate.Or(
				predicate.GenerationChangedPredicate{}, gatewayEnvironmentChanged(), gatewayXDSProgrammedChanged()))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func gatewayEnvironmentChanged() predicate.Predicate {
	return predicate.Funcs{UpdateFunc: func(update event.UpdateEvent) bool {
		return update.ObjectOld.GetLabels()[cconfig.EnvironmentLabelKey] !=
			update.ObjectNew.GetLabels()[cconfig.EnvironmentLabelKey]
	}}
}

func gatewayXDSProgrammedChanged() predicate.Predicate {
	return predicate.Funcs{UpdateFunc: func(update event.UpdateEvent) bool {
		oldGateway, oldOK := update.ObjectOld.(*gatewayv1.Gateway)
		newGateway, newOK := update.ObjectNew.(*gatewayv1.Gateway)
		if !oldOK || !newOK {
			return false
		}
		oldCondition := meta.FindStatusCondition(oldGateway.Status.Conditions, conditionTypeXDSProgrammed)
		newCondition := meta.FindStatusCondition(newGateway.Status.Conditions, conditionTypeXDSProgrammed)
		if oldCondition == nil || newCondition == nil {
			return oldCondition != nil || newCondition != nil
		}
		return oldCondition.Status != newCondition.Status ||
			oldCondition.Reason != newCondition.Reason ||
			oldCondition.ObservedGeneration != newCondition.ObservedGeneration
	}}
}

func (r *RouteReconciler) mapGatewayToRoutes(ctx context.Context, obj client.Object) []reconcile.Request {
	gateway, ok := obj.(*gatewayv1.Gateway)
	if !ok {
		return nil
	}
	routes := &gatewayv1.RouteList{}
	if err := r.List(ctx, routes, client.MatchingFields{
		IndexFieldSpecGatewayRef: types.ObjectRefFromObject(gateway).String(),
	}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list routes for Gateway reconciliation fan-out")
		return nil
	}
	requests := make([]reconcile.Request, 0, len(routes.Items))
	for i := range routes.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&routes.Items[i])})
	}
	return requests
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
