// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller // nolint: dupl

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cc "github.com/telekom/controlplane/common/pkg/controller"

	approvalreq_handler "github.com/telekom/controlplane/approval/internal/handler/approvalrequest"
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

func (r *ApprovalRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &approvalv1.ApprovalRequest{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApprovalRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("approval-request-controller")
	r.Controller = cc.NewController(&approvalreq_handler.ApprovalRequestHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&approvalv1.ApprovalRequest{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RateLimiter:             workqueue.DefaultTypedItemBasedRateLimiter[reconcile.Request](),
		}).
		Complete(r)
}
