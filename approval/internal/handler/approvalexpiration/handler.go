// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalexpiration

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/reminder"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

// Handler handles ApprovalExpiration resources
type Handler struct {
	client client.Client
}

// NewHandler creates a new ApprovalExpiration handler
func NewHandler(c client.Client) *Handler {
	return &Handler{
		client: c,
	}
}

// CreateOrUpdate processes an ApprovalExpiration resource
func (h *Handler) CreateOrUpdate(ctx context.Context, ae *v1.ApprovalExpiration) error {
	logger := log.FromContext(ctx)

	// Load parent Approval
	approval, err := h.getParentApproval(ctx, ae)
	if err != nil {
		return errors.Wrap(err, "failed to get parent approval")
	}

	// Guard: parent must be in active state (GRANTED or SUSPENDED)
	if approval.Spec.State != v1.ApprovalStateGranted && approval.Spec.State != v1.ApprovalStateSuspended {
		logger.V(1).Info("Parent approval not in expirable state, skipping",
			"approvalState", approval.Spec.State)
		return nil
	}

	now := time.Now()

	// Check if expired — transition to EXPIRED and return immediately.
	// No notification is sent here because the Approval handler's handleNotifications
	// already sends state-change notifications when it processes the GRANTED→EXPIRED transition.
	if now.After(ae.Spec.Expiration.Time) || now.Equal(ae.Spec.Expiration.Time) {
		logger.Info("Approval expired, transitioning to EXPIRED state",
			"expiration", ae.Spec.Expiration.Time)
		return h.ensureApprovalExpired(ctx, approval)
	}

	// Evaluate which reminder (if any) should fire now
	pending := reminder.Evaluate(ae.Spec.Expiration.Time, ae.Spec.Thresholds, ae.Status.SentReminders, now)
	if len(pending) > 0 {
		p := pending[0]
		logger.V(1).Info("Sending reminder", "threshold", p.Key)

		// Send notification
		notificationRef, err := h.sendReminder(ctx, ae, approval, now, p.Threshold)
		if err != nil {
			return errors.Wrap(err, "failed to send reminder")
		}

		// Track that this reminder was sent
		ae.Status.SentReminders = reminder.UpsertSent(ae.Status.SentReminders, &reminder.SentReminder{
			Threshold: p.Key,
			Ref:       *notificationRef,
			SentAt:    metav1.NewTime(now),
		})
	}

	// Schedule next reconciliation via the happy-path requeue mechanism.
	// We use SetRequeueAfter instead of returning an error to avoid the common
	// controller marking the object as NotReady and emitting Warning events.
	delay := reminder.NextRequeue(ae.Spec.Expiration.Time, ae.Spec.Thresholds, ae.Status.SentReminders, now)
	if delay <= 0 {
		delay = time.Second // Immediate retry
	}
	logger.V(1).Info("Scheduling next expiration check", "requeueAfter", delay)
	contextutil.SetRequeueAfter(ctx, delay)
	return nil
}

// Delete handles cleanup when an ApprovalExpiration is deleted
func (h *Handler) Delete(ctx context.Context, ae *v1.ApprovalExpiration) error {
	// Notifications are owned by ApprovalExpiration, auto-deleted by GC
	return nil
}

// getParentApproval fetches the parent Approval resource
func (h *Handler) getParentApproval(ctx context.Context, ae *v1.ApprovalExpiration) (*v1.Approval, error) {
	approval := &v1.Approval{}
	key := client.ObjectKey{
		Name:      ae.Spec.Approval.Name,
		Namespace: ae.Spec.Approval.Namespace,
	}
	if err := h.client.Get(ctx, key, approval); err != nil {
		return nil, err
	}
	return approval, nil
}

// ensureApprovalExpired transitions the parent Approval to EXPIRED state
func (h *Handler) ensureApprovalExpired(ctx context.Context, approval *v1.Approval) error {
	if approval.Spec.State == v1.ApprovalStateExpired {
		return nil // Already expired, no-op
	}

	// Add system Decision (matches auto-approval pattern)
	approval.Spec.Decisions = append(approval.Spec.Decisions, v1.Decision{
		Name:           v1.SystemDecisionName,
		Comment:        "Automatically expired - expiration date reached. Available actions: Allow (re-approve), Deny (reject).",
		Timestamp:      &metav1.Time{Time: time.Now()},
		ResultingState: v1.ApprovalStateExpired,
	})

	// Set state
	approval.Spec.State = v1.ApprovalStateExpired

	// Update via API server (goes through webhook)
	if err := h.client.Update(ctx, approval); err != nil {
		return errors.Wrap(err, "failed to expire approval")
	}

	return nil
}

// sendReminder sends a reminder notification for the approaching or reached expiration
func (h *Handler) sendReminder(ctx context.Context, ae *v1.ApprovalExpiration, approval *v1.Approval, now time.Time, threshold reminder.Threshold) (*types.ObjectRef, error) {
	// Calculate days remaining
	daysRemaining := int64(ae.Spec.Expiration.Time.Sub(now).Hours() / 24)
	if daysRemaining < 0 {
		daysRemaining = 0
	}

	// Determine reminder type based on threshold
	var reminderType string
	if threshold.Repeat != nil {
		reminderType = "daily" // Repeating thresholds are daily
	} else if daysRemaining == 0 {
		reminderType = "expired"
	} else {
		reminderType = "weekly" // One-shot thresholds are weekly
	}

	// Build base notification data with template properties
	notificationData := util.NotificationData{
		Owner:          ae,
		StateNew:       string(approval.Spec.State),
		StateOld:       string(approval.Spec.State), // State hasn't changed yet for reminders
		Target:         &approval.Spec.Target,
		Requester:      &approval.Spec.Requester,
		Decider:        &approval.Spec.Decider,
		Scenario:       util.NotificationScenarioUpdated,
		Action:         approval.Spec.Action,
		ExpirationDate: ae.Spec.Expiration.Format("2006-01-02"),
		DaysRemaining:  fmt.Sprintf("%d", daysRemaining),
		ReminderType:   reminderType,
	}

	// Send to decider (in decider's namespace)
	notificationData.Actor = util.ActorDecider
	notificationData.SendToChannelNamespace = approval.Spec.Decider.ApplicationRef.Namespace
	if _, err := util.SendReminderNotification(ctx, &notificationData); err != nil {
		return nil, errors.Wrap(err, "failed to send decider reminder")
	}

	// Send to requester (in requester's namespace)
	notificationData.Actor = util.ActorRequester
	notificationData.SendToChannelNamespace = approval.Spec.Requester.ApplicationRef.Namespace
	requesterRef, err := util.SendReminderNotification(ctx, &notificationData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send requester reminder")
	}

	return requesterRef, nil
}
