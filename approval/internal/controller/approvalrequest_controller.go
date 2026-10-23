// Copyright 2025 Deutsche Telekom IT GmbH
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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	approvalreq_handler "github.com/telekom/controlplane/approval/internal/handler/approvalrequest"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
)

// ApprovalRequestReconciler reconciles a ApprovalRequest object
type ApprovalRequestReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*approvalv1.ApprovalRequest]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notificationchannels,verbs=get;list;watch
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications,verbs=get;list;watch;create;update;patch;delete

func (r *ApprovalRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &approvalv1.ApprovalRequest{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApprovalRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("approval-request-controller")
	r.Controller = cc.NewController(
		&approvalreq_handler.ApprovalRequestHandler{
			Reader: mgr.GetAPIReader(),
		},
		r.Client, r.Recorder,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&approvalv1.ApprovalRequest{}, builder.WithPredicates(cc.Count("approvalrequest", cc.RoleFor))).
		Watches(&approvalv1.Approval{},
			handler.EnqueueRequestsFromMapFunc(r.mapScopedApprovalToRequests),
			builder.WithPredicates(cc.Count("approvalrequest", cc.RoleWatches,
				predicate.NewPredicateFuncs(isScopedApproval), predicate.GenerationChangedPredicate{}, ignoreDelete)),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// ignoreDelete drops delete events: a deleted scoped Approval must not wake its
// requests, which would recreate it.
var ignoreDelete = predicate.Funcs{DeleteFunc: func(event.DeleteEvent) bool { return false }}

// isScopedApproval reports whether obj is an Approval of a scoped gate.
func isScopedApproval(obj client.Object) bool {
	approval, ok := obj.(*approvalv1.Approval)
	return ok && approval.Spec.ApprovalKey != ""
}

// mapScopedApprovalToRequests maps a scoped Approval to the ApprovalRequests of
// its gate (same target and approvalKey), so that a re-granted Approval is bound
// to the gate's current request without waiting for the periodic requeue.
func (r *ApprovalRequestReconciler) mapScopedApprovalToRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	approval, ok := obj.(*approvalv1.Approval)
	if !ok || approval.Spec.ApprovalKey == "" {
		return nil
	}

	list := &approvalv1.ApprovalRequestList{}
	if err := r.List(ctx, list, client.InNamespace(approval.Namespace)); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list ApprovalRequests for scoped Approval", "approval", client.ObjectKeyFromObject(approval))
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		ar := &list.Items[i]
		if approvalv1.ScopedIdentityMatch(ar.Spec.Target, approval.Spec.Target, ar.Spec.ApprovalKey, approval.Spec.ApprovalKey) {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ar)})
		}
	}
	return reqs
}
