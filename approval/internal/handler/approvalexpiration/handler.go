// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalexpiration

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/handler/util"
	commonclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/reminder"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

// Handler handles ApprovalExpiration resources
type Handler struct{}

// NewHandler creates a new ApprovalExpiration handler
func NewHandler() *Handler {
	return &Handler{}
}

// CreateOrUpdate processes an ApprovalExpiration resource
func (h *Handler) CreateOrUpdate(ctx context.Context, ae *v1.ApprovalExpiration) error {
	logger := log.FromContext(ctx)

	// Load parent Approval
	approval, err := h.getParentApproval(ctx, ae)
	if err != nil {
		return errors.Wrap(err, "failed to get parent approval")
	}
	if approval == nil {
		// Parent Approval was deleted; this resource will be GC'd via owner reference
		return nil
	}

	// Guard: parent must be in active state (GRANTED or SUSPENDED)
	if approval.Spec.State != v1.ApprovalStateGranted && approval.Spec.State != v1.ApprovalStateSuspended {
		logger.V(1).Info("Parent approval not in expirable state, skipping",
			"approvalState", approval.Spec.State)
		return nil
	}

	now := time.Now()

	// Check if expired — log if yes (expirations are currently informational only)
	if now.After(ae.Spec.Expiration.Time) || now.Equal(ae.Spec.Expiration.Time) {
		logger.Info("Approval has expired",
			"expiration", ae.Spec.Expiration.Time)
	}

	// Evaluate which reminder (if any) should fire now
	pending := reminder.Evaluate(ae.Spec.Expiration.Time, ae.Spec.Thresholds, ae.Status.SentReminders, now)
	if len(pending) > 0 {

		// Evaluate returns at most one pending reminder (tightest threshold).
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
	logger := log.FromContext(ctx)

	approval, err := h.getParentApproval(ctx, ae)
	if err != nil {
		return errors.Wrap(err, "failed to get parent approval")
	}
	if approval == nil {
		return nil
	}

	removed := meta.RemoveStatusCondition(&approval.Status.Conditions, "Expired")
	logger.V(1).Info("Removed 'Expired' status condition", "removed", removed)

	return nil
}

// getParentApproval fetches the parent Approval resource.
// Returns nil, nil if the parent no longer exists.
func (h *Handler) getParentApproval(ctx context.Context, ae *v1.ApprovalExpiration) (*v1.Approval, error) {
	c := commonclient.ClientFromContextOrDie(ctx)
	approval := &v1.Approval{}
	key := client.ObjectKey{
		Name:      ae.Spec.Approval.Name,
		Namespace: ae.Spec.Approval.Namespace,
	}
	if err := c.Get(ctx, key, approval); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return approval, nil
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
	switch {
	case threshold.Repeat != nil:
		reminderType = "daily" // Repeating thresholds are daily
	case daysRemaining == 0:
		reminderType = "expired"
	default:
		reminderType = "weekly" // One-shot thresholds are weekly
	}

	// Build base notification data with template properties
	notificationData := util.ReminderNotificationData{
		NotificationData: util.NotificationData{
			Owner:     ae,
			StateNew:  string(approval.Spec.State),
			StateOld:  string(approval.Spec.State), // State hasn't changed yet for reminders
			Target:    &approval.Spec.Target,
			Requester: &approval.Spec.Requester,
			Decider:   &approval.Spec.Decider,
			Scenario:  util.NotificationScenarioUpdated,
			Action:    approval.Spec.Action,
		},
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
