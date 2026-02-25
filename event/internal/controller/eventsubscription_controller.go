// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventsubscription"
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

	ctypes "github.com/telekom/controlplane/common/pkg/types"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
)

// EventSubscriptionReconciler reconciles a EventSubscription object
type EventSubscriptionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*eventv1.EventSubscription]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventsubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventsubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventsubscriptions/finalizers,verbs=update
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvals,verbs=get;list;watch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventtypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=subscribers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventexposures,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch

func (r *EventSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &eventv1.EventSubscription{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("eventsubscription-controller")
	r.Controller = cc.NewController(&eventsubscription.EventSubscriptionHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&eventv1.EventSubscription{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&approvalv1.ApprovalRequest{}).
		Owns(&approvalv1.Approval{}).
		Owns(&pubsubv1.Subscriber{}).
		Watches(&eventv1.EventExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapEventExposureToEventSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&eventv1.EventConfig{},
			handler.EnqueueRequestsFromMapFunc(r.MapEventConfigToEventSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&applicationv1.Application{},
			handler.EnqueueRequestsFromMapFunc(r.MapApplicationToEventSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&adminv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneToEventSubscription),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapEventExposureToEventSubscription enqueues EventSubscriptions that are affected by changes to EventExposures.
// This is necessary to update the status of EventSubscriptions when the corresponding EventExposure changes.
func (r *EventSubscriptionReconciler) MapEventExposureToEventSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	eventExposure, ok := obj.(*eventv1.EventExposure)
	if !ok {
		return nil
	}

	list := &eventv1.EventSubscriptionList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: eventExposure.Labels[cconfig.EnvironmentLabelKey],
		eventv1.EventTypeLabelKey:   labelutil.NormalizeLabelValue(eventExposure.Spec.EventType),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if item.Spec.EventType == eventExposure.Spec.EventType {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&item),
			})
		}
	}
	return reqs
}

// MapEventConfigToEventSubscription enqueues EventSubscriptions that are affected by changes to EventConfigs.
// This is necessary to update the status of EventSubscriptions when the corresponding EventConfig changes.
func (r *EventSubscriptionReconciler) MapEventConfigToEventSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	eventConfig, ok := obj.(*eventv1.EventConfig)
	if !ok {
		return nil
	}

	list := &eventv1.EventSubscriptionList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   eventConfig.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("zone"): labelutil.NormalizeLabelValue(eventConfig.Spec.Zone.Name),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if !item.Spec.Zone.Equals(&eventConfig.Spec.Zone) {
			continue
		}
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&item),
		})
	}
	return reqs
}

// MapApplicationToEventSubscription enqueues EventSubscriptions that are affected by changes to Applications.
// This is necessary to update the status of EventSubscriptions when the corresponding Application is updated, e.g. becoming ready
func (r *EventSubscriptionReconciler) MapApplicationToEventSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	application, ok := obj.(*applicationv1.Application)
	if !ok {
		return nil
	}

	list := &eventv1.EventSubscriptionList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:          application.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("application"): labelutil.NormalizeLabelValue(application.Name),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, item := range list.Items {
		if !ctypes.ObjectRefFromObject(application).Equals(&item.Spec.Requestor) {
			continue
		}

		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&item),
		})
	}
	return reqs

}

func (r *EventSubscriptionReconciler) MapZoneToEventSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	list := &eventv1.EventSubscriptionList{}
	if err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   zone.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("zone"): labelutil.NormalizeLabelValue(zone.Name),
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
