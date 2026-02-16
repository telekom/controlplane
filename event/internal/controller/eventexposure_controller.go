// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventexposure"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
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

// EventExposureReconciler reconciles a EventExposure object
type EventExposureReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*eventv1.EventExposure]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventexposures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventexposures/finalizers,verbs=update
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms,verbs=get;list;watch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventsubscriptions,verbs=get;list;watch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventtypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=eventstores,verbs=get;list;watch
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch

func (r *EventExposureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &eventv1.EventExposure{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventExposureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("eventexposure-controller")
	r.Controller = cc.NewController(&eventexposure.EventExposureHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&eventv1.EventExposure{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Owns(&pubsubv1.Publisher{}).
		Watches(&gatewayv1.Route{},
			handler.EnqueueRequestsFromMapFunc(r.MapRouteToEventExposure),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}, LabelPredicate),
		).
		Watches(&eventv1.EventType{},
			handler.EnqueueRequestsFromMapFunc(r.MapEventTypeToEventExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&eventv1.EventExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapEventExposureToEventExposure),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&adminv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneToEventExposure),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&eventv1.EventConfig{},
			handler.EnqueueRequestsFromMapFunc(r.MapEventConfigToEventExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&eventv1.EventSubscription{},
			handler.EnqueueRequestsFromMapFunc(r.MapEventSubscriptionToEventExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapEventTypeToEventExposure enqueues EventExposures referencing the changed EventType.
// This ensures exposures react to changes in the EventType (e.g., schema changes).
func (r *EventExposureReconciler) MapEventTypeToEventExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	eventType, ok := obj.(*eventv1.EventType)
	if !ok {
		return nil
	}

	list := &eventv1.EventExposureList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: eventType.Labels[cconfig.EnvironmentLabelKey],
		eventv1.EventTypeLabelKey:   labelutil.NormalizeLabelValue(eventType.Spec.Type),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if item.Spec.EventType == eventType.Spec.Type {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&item),
			})
		}
	}
	return reqs
}

// MapEventExposureToEventExposure enqueues other EventExposures with the same event type
// when any EventExposure changes or is deleted. This triggers standby exposures to detect
// the active one is gone and become active themselves.
func (r *EventExposureReconciler) MapEventExposureToEventExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	exposure, ok := obj.(*eventv1.EventExposure)
	if !ok {
		return nil
	}

	list := &eventv1.EventExposureList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: exposure.Labels[cconfig.EnvironmentLabelKey],
		eventv1.EventTypeLabelKey:   labelutil.NormalizeLabelValue(exposure.Spec.EventType),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if item.UID == exposure.UID {
			continue
		}
		if item.Spec.EventType == exposure.Spec.EventType {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&item),
			})
		}
	}
	return reqs
}

// MapRouteToEventExposure enqueues EventExposures whose event type matches
// the Route's EventTypeLabelKey label. This allows EventExposure to react
// to Route status changes (e.g., Route becoming ready).
func (r *EventExposureReconciler) MapRouteToEventExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*gatewayv1.Route)
	if !ok {
		return nil
	}

	// Only care about SSE Routes (identified by EventTypeLabelKey)
	eventTypeLabel := route.GetLabels()[eventv1.EventTypeLabelKey]
	if eventTypeLabel == "" {
		return nil
	}

	list := &eventv1.EventExposureList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: route.Labels[cconfig.EnvironmentLabelKey],
		eventv1.EventTypeLabelKey:   eventTypeLabel,
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		normalized := labelutil.NormalizeLabelValue(item.Spec.EventType)
		if normalized == eventTypeLabel {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&item),
			})
		}
	}
	return reqs
}

// MapZoneToEventExposure enqueues EventExposures referencing the changed Zone.
// This ensures EventExposures react to zone status changes (e.g., GatewayRealm becoming available).
func (r *EventExposureReconciler) MapZoneToEventExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	list := &eventv1.EventExposureList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   zone.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("zone"): labelutil.NormalizeLabelValue(zone.Name),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if item.Spec.Zone.Name == zone.Name {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&item),
			})
		}
	}
	return reqs
}

// MapEventConfigToEventExposure enqueues EventExposures referencing the same zone as the changed EventConfig.
// This ensures EventExposures react to EventConfig changes (e.g., SSE URL updates, config becoming ready).
func (r *EventExposureReconciler) MapEventConfigToEventExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	eventConfig, ok := obj.(*eventv1.EventConfig)
	if !ok {
		return nil
	}

	// TODO: full environment list --> investigate if we can optimize it
	list := &eventv1.EventExposureList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: eventConfig.Labels[cconfig.EnvironmentLabelKey],
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if item.Spec.Zone.Equals(&eventConfig.Spec.Zone) {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&item),
			})
		}
	}
	return reqs
}

// MapEventSubscriptionToEventExposure enqueues EventExposures whose event type matches
// the subscription's event type. This triggers proxy route creation/cleanup when SSE
// subscriptions are created, updated, or deleted.
func (r *EventExposureReconciler) MapEventSubscriptionToEventExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	sub, ok := obj.(*eventv1.EventSubscription)
	if !ok {
		return nil
	}

	// Only care about SSE subscriptions — callback subscriptions don't need proxy routes
	if sub.Spec.Delivery.Type != eventv1.DeliveryTypeServerSentEvent {
		return nil
	}

	list := &eventv1.EventExposureList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: sub.Labels[cconfig.EnvironmentLabelKey],
		eventv1.EventTypeLabelKey:   labelutil.NormalizeLabelValue(sub.Spec.EventType),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if item.Spec.EventType == sub.Spec.EventType {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&item),
			})
		}
	}
	return reqs
}
