// Copyright 2026 Deutsche Telekom IT GmbH
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	agenticconfig "github.com/telekom/controlplane/agentic/internal/config"
	"github.com/telekom/controlplane/agentic/internal/handler/agenticexposure"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// AgenticExposureReconciler reconciles a AgenticExposure object
type AgenticExposureReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   *agenticconfig.AgenticConfig

	cc.Controller[*agenticv1.AgenticExposure]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticexposures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticexposures/finalizers,verbs=update
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticsubscriptions,verbs=get;list;watch
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch

func (r *AgenticExposureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &agenticv1.AgenticExposure{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgenticExposureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("agenticexposure-controller")
	r.Controller = cc.NewController(&agenticexposure.AgenticExposureHandler{Config: r.Config}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&agenticv1.AgenticExposure{}).
		Watches(&gatewayv1.Route{},
			handler.EnqueueRequestsFromMapFunc(r.MapRouteToAgenticExposure),
			builder.WithPredicates(LabelPredicate),
		).
		Watches(&agenticv1.AgenticServer{},
			handler.EnqueueRequestsFromMapFunc(r.MapAgenticServerToAgenticExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&agenticv1.AgenticExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapAgenticExposureToAgenticExposure),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&adminv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneToAgenticExposure),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&agenticv1.AgenticSubscription{},
			handler.EnqueueRequestsFromMapFunc(r.MapAgenticSubscriptionToAgenticExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapAgenticServerToAgenticExposure enqueues AgenticExposures referencing the changed AgenticServer.
func (r *AgenticExposureReconciler) MapAgenticServerToAgenticExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	server, ok := obj.(*agenticv1.AgenticServer)
	if !ok {
		return nil
	}

	list := &agenticv1.AgenticExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:       server.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(server.Spec.BasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.BasePath == server.Spec.BasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}

// MapAgenticExposureToAgenticExposure enqueues other AgenticExposures with the same basePath.
//
//nolint:dupl // parallel structure with MapAgenticServerToAgenticServer; operates on different types
func (r *AgenticExposureReconciler) MapAgenticExposureToAgenticExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	exposure, ok := obj.(*agenticv1.AgenticExposure)
	if !ok {
		return nil
	}

	list := &agenticv1.AgenticExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:       exposure.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(exposure.Spec.BasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].UID == exposure.UID {
			continue
		}
		if list.Items[i].Spec.BasePath == exposure.Spec.BasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}

// MapRouteToAgenticExposure enqueues AgenticExposures whose basePath matches the Route's label.
func (r *AgenticExposureReconciler) MapRouteToAgenticExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*gatewayv1.Route)
	if !ok {
		return nil
	}

	basePathLabel := route.GetLabels()[agenticv1.AgenticBasePathLabelKey]
	if basePathLabel == "" {
		return nil
	}

	list := &agenticv1.AgenticExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:       route.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.AgenticBasePathLabelKey: basePathLabel,
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		normalized := labelutil.NormalizeLabelValue(list.Items[i].Spec.BasePath)
		if normalized == basePathLabel {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}

// MapZoneToAgenticExposure enqueues AgenticExposures referencing the changed Zone.
func (r *AgenticExposureReconciler) MapZoneToAgenticExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	list := &agenticv1.AgenticExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   zone.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("zone"): labelutil.NormalizeLabelValue(zone.Name),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.Zone.Name == zone.Name {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}

// MapAgenticSubscriptionToAgenticExposure enqueues AgenticExposures whose basePath matches
// the subscription's basePath. This triggers proxy route creation/cleanup.
func (r *AgenticExposureReconciler) MapAgenticSubscriptionToAgenticExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	sub, ok := obj.(*agenticv1.AgenticSubscription)
	if !ok {
		return nil
	}

	list := &agenticv1.AgenticExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:       sub.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(sub.Spec.BasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.BasePath == sub.Spec.BasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}
