// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/handler/approvalexpiration"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
)

// ApprovalExpirationReconciler reconciles an ApprovalExpiration object
type ApprovalExpirationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder // Deprecated: kept for common controller compatibility
	EventRecorder events.EventRecorder // New events API

	cc.Controller[*approvalv1.ApprovalExpiration]
}

// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalexpirations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalexpirations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalexpirations/finalizers,verbs=update
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvals,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notificationchannels,verbs=get;list;watch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *ApprovalExpirationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &approvalv1.ApprovalExpiration{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApprovalExpirationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Use new events API (k8s.io/client-go/tools/events)
	r.EventRecorder = mgr.GetEventRecorder("approvalexpiration-controller")
	// TODO: Migrate common controller to use events.EventRecorder, then remove this adapter
	r.Recorder = &eventsRecorderAdapter{events: r.EventRecorder}

	handler := approvalexpiration.NewHandler(r.Client)
	r.Controller = cc.NewController(handler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&approvalv1.ApprovalExpiration{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// eventsRecorderAdapter adapts events.EventRecorder (new API) to record.EventRecorder (old API)
// for compatibility with common controller until it's migrated to the new API.
type eventsRecorderAdapter struct {
	events events.EventRecorder
}

func (a *eventsRecorderAdapter) Event(object runtime.Object, eventtype, reason, message string) {
	a.events.Eventf(object, nil, eventtype, reason, "Event", message)
}

func (a *eventsRecorderAdapter) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	a.events.Eventf(object, nil, eventtype, reason, "Event", messageFmt, args...)
}

func (a *eventsRecorderAdapter) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	// events.EventRecorder doesn't support annotations in the same way, just forward to Eventf
	a.events.Eventf(object, nil, eventtype, reason, "Event", messageFmt, args...)
}
