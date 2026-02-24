// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventconfig"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
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

// EventConfigReconciler reconciles a EventConfig object
type EventConfigReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*eventv1.EventConfig]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms,verbs=get;list;watch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=eventstores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms,verbs=get;list;watch

func (r *EventConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &eventv1.EventConfig{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("eventconfig-controller")
	r.Controller = cc.NewController(&eventconfig.EventConfigHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&eventv1.EventConfig{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Owns(&pubsubv1.EventStore{}).
		Owns(&gatewayv1.Route{}, builder.WithPredicates(LabelPredicate)).
		Owns(&identityv1.Client{}, builder.WithPredicates(LabelPredicate)).
		Watches(&adminv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneToEventConfig),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&eventv1.EventConfig{},
			handler.EnqueueRequestsFromMapFunc(r.MapEventConfigToEventConfig),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapZoneToEventConfig enqueues EventConfig referencing the changed Zone.
func (r *EventConfigReconciler) MapZoneToEventConfig(ctx context.Context, obj client.Object) []reconcile.Request {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	list := &eventv1.EventConfigList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: zone.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if !item.Spec.Zone.Equals(zone) {
			continue
		}
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&item),
		})
	}
	return reqs
}

// MapEventConfigToEventConfig enqueues other EventConfig referencing the changed EventConfig.
// This is required to trigger updates for meshing
func (r *EventConfigReconciler) MapEventConfigToEventConfig(ctx context.Context, obj client.Object) []reconcile.Request {
	eventConfig, ok := obj.(*eventv1.EventConfig)
	if !ok {
		return nil
	}

	list := &eventv1.EventConfigList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: eventConfig.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if ctypes.Equals(&item, eventConfig) {
			continue
		}
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&item),
		})

	}
	return reqs
}
