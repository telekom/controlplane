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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/agenticsubscription"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// AgenticSubscriptionReconciler reconciles a AgenticSubscription object
type AgenticSubscriptionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*agenticv1.AgenticSubscription]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticsubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticsubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticsubscriptions/finalizers,verbs=update
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticexposures,verbs=get;list;watch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumeroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=get;list;watch;create;update;patch;delete

func (r *AgenticSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &agenticv1.AgenticSubscription{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgenticSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("agenticsubscription-controller")
	r.Controller = cc.NewController(&agenticsubscription.AgenticSubscriptionHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&agenticv1.AgenticSubscription{}).
		Owns(&gatewayv1.ConsumeRoute{}).
		Owns(&approvalv1.ApprovalRequest{}).
		Watches(&agenticv1.AgenticExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapAgenticExposureToAgenticSubscription),
		    builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&approvalv1.Approval{},
			handler.EnqueueRequestsFromMapFunc(r.MapApprovalToAgenticSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&applicationv1.Application{},
			handler.EnqueueRequestsFromMapFunc(r.MapApplicationToMcpSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&gatewayv1.Route{},
			handler.EnqueueRequestsFromMapFunc(r.MapRouteToMcpSubscription),
			builder.WithPredicates(cc.DeleteOnlyPredicate{}),
		).
		Watches(&adminv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneToMcpSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapAgenticExposureToAgenticSubscription enqueues AgenticSubscriptions whose basePath matches
// the changed AgenticExposure.
func (r *AgenticSubscriptionReconciler) MapAgenticExposureToAgenticSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	exposure, ok := obj.(*agenticv1.AgenticExposure)
	if !ok {
		return nil
	}

	list := &agenticv1.AgenticSubscriptionList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:       exposure.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(exposure.Spec.BasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].Spec.BasePath == exposure.Spec.BasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}

// MapApprovalToAgenticSubscription enqueues AgenticSubscriptions that own the changed Approval.
func (r *AgenticSubscriptionReconciler) MapApprovalToAgenticSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	approval, ok := obj.(*approvalv1.Approval)
	if !ok {
		return nil
	}

	// Get owner reference
	for _, ref := range approval.GetOwnerReferences() {
		if ref.Kind == "AgenticSubscription" {
			return []reconcile.Request{
				{NamespacedName: client.ObjectKey{
					Name:      ref.Name,
					Namespace: approval.Namespace,
				}},
			}
		}
	}
	return nil
}

// MapApplicationToMcpSubscription enqueues McpSubscriptions that reference the changed Application.
func (r *McpSubscriptionReconciler) MapApplicationToMcpSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	application, ok := obj.(*applicationv1.Application)
	if !ok {
		logger.Info("object is not an Application")
		return nil
	}

	list := &agenticv1.McpSubscriptionList{}
	err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:          application.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("application"): labelutil.NormalizeLabelValue(application.Name),
	}, client.InNamespace(application.Namespace))
	if err != nil {
		logger.Error(err, "failed to list McpSubscriptions")
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
		})
	}

	return reqs
}

// MapRouteToMcpSubscription enqueues McpSubscriptions when a Route they reference is deleted.
func (r *McpSubscriptionReconciler) MapRouteToMcpSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	route, ok := obj.(*gatewayv1.Route)
	if !ok {
		return nil
	}

	basePathLabel := route.Labels[agenticv1.McpBasePathLabelKey]
	if basePathLabel == "" {
		return nil
	}

	list := &agenticv1.McpSubscriptionList{}
	err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   route.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.McpBasePathLabelKey: basePathLabel,
	})
	if err != nil {
		logger.Error(err, "failed to list McpSubscriptions for Route")
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
		})
	}
	return reqs
}

// MapZoneToMcpSubscription enqueues McpSubscriptions that reference a changed Zone.
// This ensures subscriptions react to zone feature or visibility changes.
func (r *McpSubscriptionReconciler) MapZoneToMcpSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	list := &agenticv1.McpSubscriptionList{}
	err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   zone.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("zone"): labelutil.NormalizeLabelValue(zone.Name),
	})
	if err != nil {
		logger.Error(err, "failed to list McpSubscriptions for Zone")
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].Spec.Zone.Name == zone.Name {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}
