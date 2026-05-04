// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/config"
	"github.com/telekom/controlplane/approval/internal/handler/approvalexpiration"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
)

// ApprovalExpirationReconciler reconciles an ApprovalExpiration object
type ApprovalExpirationReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
	ExpirationConfig *config.ExpirationConfig

	cc.Controller[*approvalv1.ApprovalExpiration]
}

// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalexpirations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalexpirations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalexpirations/finalizers,verbs=update
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvals,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notificationchannels,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *ApprovalExpirationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &approvalv1.ApprovalExpiration{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApprovalExpirationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("approvalexpiration-controller")
	handler := approvalexpiration.NewHandler(r.Client, r.ExpirationConfig)
	r.Controller = cc.NewController(handler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&approvalv1.ApprovalExpiration{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
