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

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/mcpsubscription"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// McpSubscriptionReconciler reconciles a McpSubscription object
type McpSubscriptionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*agenticv1.McpSubscription]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpsubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpsubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpsubscriptions/finalizers,verbs=update
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpexposures,verbs=get;list;watch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumeroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=get;list;watch;create;update;patch;delete

func (r *McpSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &agenticv1.McpSubscription{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *McpSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("mcpsubscription-controller")
	r.Controller = cc.NewController(&mcpsubscription.McpSubscriptionHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&agenticv1.McpSubscription{}).
		Owns(&gatewayv1.ConsumeRoute{}).
		Watches(&agenticv1.McpExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapMcpExposureToMcpSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&approvalv1.Approval{},
			handler.EnqueueRequestsFromMapFunc(r.MapApprovalToMcpSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapMcpExposureToMcpSubscription enqueues McpSubscriptions whose basePath matches
// the changed McpExposure.
func (r *McpSubscriptionReconciler) MapMcpExposureToMcpSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	exposure, ok := obj.(*agenticv1.McpExposure)
	if !ok {
		return nil
	}

	list := &agenticv1.McpSubscriptionList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   exposure.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(exposure.Spec.BasePath),
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

// MapApprovalToMcpSubscription enqueues McpSubscriptions that own the changed Approval.
func (r *McpSubscriptionReconciler) MapApprovalToMcpSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	approval, ok := obj.(*approvalv1.Approval)
	if !ok {
		return nil
	}

	// Get owner reference
	for _, ref := range approval.GetOwnerReferences() {
		if ref.Kind == "McpSubscription" {
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
