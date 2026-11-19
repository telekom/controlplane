// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	routehandler "github.com/telekom/controlplane/gateway/internal/handler/route"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

// RouteReconciler reconciles a Route object
type RouteReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	// KongClients is the operator wide Kong client factory.
	KongClients *kongutil.ClientFactory

	cc.Controller[*gatewayv1.Route]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes/finalizers,verbs=update

func (r *RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &gatewayv1.Route{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.KongClients == nil {
		return errors.New("kong client factory is required")
	}
	r.Recorder = mgr.GetEventRecorderFor("route-controller")
	r.Controller = cc.NewController(&routehandler.RouteHandler{KongClients: r.KongClients}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Route{}, builder.WithPredicates(cc.Count("route", cc.RoleFor,
			predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{})))).
		Watches(&gatewayv1.ConsumeRoute{},
			handler.EnqueueRequestsFromMapFunc(r.mapConsumeRouteToRoute),
			builder.WithPredicates(cc.Count("route", cc.RoleWatches, predicate.GenerationChangedPredicate{}, cc.SkipInitialListPredicate{}))).
		Watches(&gatewayv1.RouteListener{},
			handler.EnqueueRequestsFromMapFunc(r.mapRouteListenerToRoute),
			builder.WithPredicates(cc.Count("route", cc.RoleWatches, predicate.GenerationChangedPredicate{}, cc.SkipInitialListPredicate{}))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *RouteReconciler) mapConsumeRouteToRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	consumeRoute, ok := obj.(*gatewayv1.ConsumeRoute)
	if !ok {
		return nil
	}
	log := logr.FromContextOrDiscard(ctx)

	direct := consumeRoute.Spec.Route.K8s()
	requests := []reconcile.Request{{NamespacedName: direct}}

	env := consumeRoute.Labels[cconfig.EnvironmentLabelKey]
	if env == "" {
		return requests
	}

	// Primary and failover-secondary Routes build their JumperConfig from all ConsumeRoutes
	// with the same route name, including cross-zone ones that reference a proxy Route.
	// Mirror that lookup, restricted to the ConsumeRoute's environment.
	routes := &gatewayv1.RouteList{}
	if err := r.List(ctx, routes,
		client.MatchingFields{IndexFieldMetadataName: consumeRoute.Spec.Route.Name},
		client.MatchingLabels{cconfig.EnvironmentLabelKey: env},
	); err != nil {
		log.Error(err, "Failed to list Routes by name", "routeName", consumeRoute.Spec.Route.Name)
		return requests
	}

	for i := range routes.Items {
		route := &routes.Items[i]
		if route.IsProxy() && !route.IsFailoverSecondary() {
			continue
		}
		key := client.ObjectKeyFromObject(route)
		if key == direct {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: key})
	}
	return requests
}

func (r *RouteReconciler) mapRouteListenerToRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	routeListener, ok := obj.(*gatewayv1.RouteListener)
	if !ok {
		return nil
	}

	// The RouteListener's spec.route references the Route it attaches to.
	route := &gatewayv1.Route{}
	if err := r.Get(ctx, routeListener.Spec.Route.K8s(), route); err != nil {
		return nil
	}

	return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: route.Name, Namespace: route.Namespace}}}
}
