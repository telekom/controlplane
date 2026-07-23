// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	consumer_handler "github.com/telekom/controlplane/gateway/internal/handler/consumer"
)

// ConsumerReconciler reconciles a Consumer object
type ConsumerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*gatewayv1.Consumer]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumers/finalizers,verbs=update

func (r *ConsumerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &gatewayv1.Consumer{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConsumerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("consumer-controller")
	r.Controller = cc.NewController(&consumer_handler.ConsumerHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Consumer{}).
		Watches(&gatewayv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.mapGatewayToConsumers),
			builder.WithPredicates(predicate.Or(
				predicate.GenerationChangedPredicate{}, gatewayEnvironmentChanged(), gatewayXDSProgrammedChanged()))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *ConsumerReconciler) mapGatewayToConsumers(ctx context.Context, obj client.Object) []reconcile.Request {
	gateway, ok := obj.(*gatewayv1.Gateway)
	if !ok {
		return nil
	}
	consumers := &gatewayv1.ConsumerList{}
	if err := r.List(ctx, consumers, client.MatchingFields{
		IndexFieldSpecGateway: types.ObjectRefFromObject(gateway).String(),
	}); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list consumers for Gateway reconciliation fan-out")
		return nil
	}
	requests := make([]reconcile.Request, 0, len(consumers.Items))
	for i := range consumers.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&consumers.Items[i])})
	}
	return requests
}
