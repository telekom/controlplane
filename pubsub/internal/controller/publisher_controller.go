// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/handler/publisher"
	"github.com/telekom/controlplane/pubsub/internal/index"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// PublisherReconciler reconciles a Publisher object
type PublisherReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*pubsubv1.Publisher]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers/finalizers,verbs=update
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=eventstores,verbs=get;list;watch

func (r *PublisherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &pubsubv1.Publisher{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *PublisherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("publisher-controller")
	r.Controller = cc.NewController(&publisher.PublisherHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&pubsubv1.Publisher{}).
		Watches(&pubsubv1.EventStore{},
			handler.EnqueueRequestsFromMapFunc(r.MapEventStoreToPublisher),
			builder.WithPredicates(),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *PublisherReconciler) MapEventStoreToPublisher(ctx context.Context, obj client.Object) []reconcile.Request {
	eventStore, ok := obj.(*pubsubv1.EventStore)
	if !ok {
		return nil
	}

	var publishers pubsubv1.PublisherList
	err := r.List(ctx, &publishers,
		client.InNamespace(eventStore.Namespace),
		client.MatchingFields{index.PublisherEventStoreIndex: eventStore.Name},
	)
	if err != nil {
		ctrl.Log.Error(err, "unable to list publishers for event store", "eventStore", eventStore.Name)
		return nil
	}

	var requests []reconcile.Request
	for _, publisher := range publishers.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name: publisher.Name,
			},
		})
	}

	return requests
}
