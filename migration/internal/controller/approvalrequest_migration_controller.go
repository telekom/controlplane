// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Handler interface for processing ApprovalRequests
type Handler interface {
	Handle(ctx context.Context, approvalRequest *approvalv1.ApprovalRequest) error
}

const (
	// RequeueAfterDuration is the time to wait before requeuing
	RequeueAfterDuration = 30 * time.Second
)

// ApprovalRequestMigrationReconciler reconciles ApprovalRequest objects for migration
type ApprovalRequestMigrationReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Handler Handler
	Log     logr.Logger
}

// NewApprovalRequestMigrationReconciler creates a new reconciler
func NewApprovalRequestMigrationReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	handler Handler,
	log logr.Logger,
) *ApprovalRequestMigrationReconciler {
	return &ApprovalRequestMigrationReconciler{
		Client:  client,
		Scheme:  scheme,
		Handler: handler,
		Log:     log,
	}
}

// Reconcile handles the reconciliation of ApprovalRequest resources
func (r *ApprovalRequestMigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling ApprovalRequest for migration", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ApprovalRequest
	approvalRequest := &approvalv1.ApprovalRequest{}
	if err := r.Get(ctx, req.NamespacedName, approvalRequest); err != nil {
		// Object not found, might have been deleted
		log.V(1).Info("ApprovalRequest not found, likely deleted")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Processing ApprovalRequest",
		"state", approvalRequest.Spec.State,
		"hasOwnerRef", len(approvalRequest.OwnerReferences) > 0)

	// Process the migration
	if err := r.Handler.Handle(ctx, approvalRequest); err != nil {
		log.Error(err, "Failed to handle migration")
		// Requeue with delay on error
		return ctrl.Result{RequeueAfter: RequeueAfterDuration}, err
	}

	log.V(1).Info("Successfully processed ApprovalRequest, requeuing for periodic check")
	// Requeue periodically to check for state changes
	return ctrl.Result{RequeueAfter: RequeueAfterDuration}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ApprovalRequestMigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&approvalv1.ApprovalRequest{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
