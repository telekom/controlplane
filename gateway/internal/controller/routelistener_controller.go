// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	routelistener_handler "github.com/telekom/controlplane/gateway/internal/handler/routelistener"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RouteListenerReconciler reconciles a RouteListener object
type RouteListenerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*gatewayv1.RouteListener]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routelisteners,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routelisteners/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routelisteners/finalizers,verbs=update

func (r *RouteListenerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &gatewayv1.RouteListener{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RouteListenerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("routelistener-controller")
	r.Controller = cc.NewController(&routelistener_handler.RouteListenerHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.RouteListener{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Watches(&gatewayv1.Route{},
			handler.EnqueueRequestsFromMapFunc(r.mapRouteToRouteListener),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Complete(r)
}

func (r *RouteListenerReconciler) mapRouteToRouteListener(ctx context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*gatewayv1.Route)
	if !ok {
		return nil
	}
	if route.Labels == nil {
		return nil
	}

	listOpts := []client.ListOption{
		client.MatchingFields{
			IndexFieldSpecRoute: types.ObjectRefFromObject(route).String(),
		},
		client.MatchingLabels{
			cconfig.EnvironmentLabelKey: route.Labels[cconfig.EnvironmentLabelKey],
		},
	}

	list := gatewayv1.RouteListenerList{}
	if err := r.List(ctx, &list, listOpts...); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		if item.Spec.Route.Equals(route) {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: item.Name, Namespace: item.Namespace}})
		}
	}

	return requests
}
