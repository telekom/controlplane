// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/handler/subscriber"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SubscriberReconciler reconciles a Subscriber object
type SubscriberReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*pubsubv1.Subscriber]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=subscribers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=subscribers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=subscribers/finalizers,verbs=update
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=eventstores,verbs=get;list;watch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers,verbs=get;list;watch

func (r *SubscriberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &pubsubv1.Subscriber{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *SubscriberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("subscriber-controller")
	r.Controller = cc.NewController(&subscriber.SubscriberHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&pubsubv1.Subscriber{}).
		Watches(&pubsubv1.Publisher{},
			handler.EnqueueRequestsFromMapFunc(r.MapPublisherToSubscriber),
			builder.WithPredicates(),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *SubscriberReconciler) MapPublisherToSubscriber(ctx context.Context, obj client.Object) []reconcile.Request {
	publisher, ok := obj.(*pubsubv1.Publisher)
	if !ok {
		return nil
	}

	list := &pubsubv1.SubscriberList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:        publisher.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("eventtype"): publisher.Labels[cconfig.BuildLabelKey("eventtype")],
	}); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, len(list.Items))
	for i, subscriber := range list.Items {
		if !subscriber.Spec.Publisher.Equals(publisher) {
			continue
		}
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      subscriber.Name,
				Namespace: subscriber.Namespace,
			},
		}
	}

	return requests
}
