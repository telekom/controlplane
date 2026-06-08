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
	"github.com/telekom/controlplane/agentic/internal/handler/mcpexposure"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// McpExposureReconciler reconciles a McpExposure object
type McpExposureReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*agenticv1.McpExposure]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpexposures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpexposures/finalizers,verbs=update
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumeroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpsubscriptions,verbs=get;list;watch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch

func (r *McpExposureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &agenticv1.McpExposure{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *McpExposureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("mcpexposure-controller")
	r.Controller = cc.NewController(&mcpexposure.McpExposureHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&agenticv1.McpExposure{}).
		Watches(&gatewayv1.Route{},
			handler.EnqueueRequestsFromMapFunc(r.MapRouteToMcpExposure),
			builder.WithPredicates(LabelPredicate),
		).
		Watches(&agenticv1.McpServer{},
			handler.EnqueueRequestsFromMapFunc(r.MapMcpServerToMcpExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&agenticv1.McpExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapMcpExposureToMcpExposure),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&adminv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneToMcpExposure),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&agenticv1.McpSubscription{},
			handler.EnqueueRequestsFromMapFunc(r.MapMcpSubscriptionToMcpExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapMcpServerToMcpExposure enqueues McpExposures referencing the changed McpServer.
func (r *McpExposureReconciler) MapMcpServerToMcpExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	server, ok := obj.(*agenticv1.McpServer)
	if !ok {
		return nil
	}

	list := &agenticv1.McpExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   server.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(server.Spec.BasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.McpBasePath == server.Spec.BasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}

// MapMcpExposureToMcpExposure enqueues other McpExposures with the same basePath.
func (r *McpExposureReconciler) MapMcpExposureToMcpExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	exposure, ok := obj.(*agenticv1.McpExposure)
	if !ok {
		return nil
	}

	list := &agenticv1.McpExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   exposure.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(exposure.Spec.McpBasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].UID == exposure.UID {
			continue
		}
		if list.Items[i].Spec.McpBasePath == exposure.Spec.McpBasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}

// MapRouteToMcpExposure enqueues McpExposures whose basePath matches the Route's label.
func (r *McpExposureReconciler) MapRouteToMcpExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*gatewayv1.Route)
	if !ok {
		return nil
	}

	basePathLabel := route.GetLabels()[agenticv1.McpBasePathLabelKey]
	if basePathLabel == "" {
		return nil
	}

	list := &agenticv1.McpExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   route.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.McpBasePathLabelKey: basePathLabel,
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		normalized := labelutil.NormalizeLabelValue(list.Items[i].Spec.McpBasePath)
		if normalized == basePathLabel {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}

// MapZoneToMcpExposure enqueues McpExposures referencing the changed Zone.
func (r *McpExposureReconciler) MapZoneToMcpExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	list := &agenticv1.McpExposureList{}
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

// MapMcpSubscriptionToMcpExposure enqueues McpExposures whose basePath matches
// the subscription's basePath. This triggers proxy route creation/cleanup.
func (r *McpExposureReconciler) MapMcpSubscriptionToMcpExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	sub, ok := obj.(*agenticv1.McpSubscription)
	if !ok {
		return nil
	}

	list := &agenticv1.McpExposureList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   sub.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(sub.Spec.McpBasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.McpBasePath == sub.Spec.McpBasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}
