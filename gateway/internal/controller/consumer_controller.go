// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	consumer_handler "github.com/telekom/controlplane/gateway/internal/handler/consumer"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

// ConsumerReconciler reconciles a Consumer object
type ConsumerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// ClientFactory acquires Kong clients; it is required.
	ClientFactory kongutil.ClientFactory

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
	if r.ClientFactory == nil {
		return errors.New("setting up consumer controller: kong client factory is required")
	}
	r.Recorder = mgr.GetEventRecorderFor("consumer-controller")
	r.Controller = cc.NewController(&consumer_handler.ConsumerHandler{Factory: r.ClientFactory}, r.Client, r.Recorder)

	fanout := &gatewayFanout{client: r.Client}

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Consumer{}, builder.WithPredicates(cc.Count("consumer", cc.RoleFor,
			predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{})))).
		// Gateway changes enqueue active and terminating dependents.
		Watches(&gatewayv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(fanout.MapGatewayToConsumers),
			builder.WithPredicates(gatewayChanged)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
