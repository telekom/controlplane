// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	crhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/telekom/controlplane/gateway/api/v1"
	handler "github.com/telekom/controlplane/gateway/internal/handler/gateway"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*v1.Gateway]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=gateways/finalizers,verbs=update

func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &v1.Gateway{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("gateway-controller")
	r.Controller = cc.NewController(&handler.GatewayHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Gateway{}).
		Watches(&v1.Route{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapRouteToGateway),
			builder.WithPredicates(cc.DeleteOnlyPredicate{})).
		Watches(&v1.Consumer{},
			crhandler.EnqueueRequestsFromMapFunc(r.mapConsumerToGateway),
			builder.WithPredicates(cc.DeleteOnlyPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *GatewayReconciler) mapConsumerToGateway(_ context.Context, obj client.Object) []reconcile.Request {
	consumer, ok := obj.(*v1.Consumer)
	if !ok || consumer.Spec.Gateway.IsEmpty() {
		return nil
	}
	return []reconcile.Request{{NamespacedName: consumer.Spec.Gateway.K8s()}}
}

func (r *GatewayReconciler) mapRouteToGateway(_ context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*v1.Route)
	if !ok || route.Spec.GatewayRef.IsEmpty() {
		return nil
	}
	return []reconcile.Request{{NamespacedName: route.Spec.GatewayRef.K8s()}}
}
