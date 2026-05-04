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
	"github.com/telekom/controlplane/approval/internal/config"
	"github.com/telekom/controlplane/approval/internal/handler/util"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

// Handler handles ApprovalExpiration resources
type Handler struct {
	client client.Client
	config *config.ExpirationConfig
}

// NewHandler creates a new ApprovalExpiration handler
func NewHandler(c client.Client, cfg *config.ExpirationConfig) *Handler {
	return &Handler{
		client: c,
		config: cfg,
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

	// Check if expired
	if now.After(ae.Spec.Expiration.Time) || now.Equal(ae.Spec.Expiration.Time) {
		logger.Info("Approval expired, transitioning to EXPIRED state",
			"expiration", ae.Spec.Expiration.Time)
		return h.ensureApprovalExpired(ctx, approval)
	}

	// Check if reminder should be sent
	if h.shouldRemind(ae, now) {
		if err := h.sendReminder(ctx, ae, approval, now); err != nil {
			return errors.Wrap(err, "failed to send reminder")
		}
		ae.Status.LastReminder = &metav1.Time{Time: now}
		logger.V(1).Info("Reminder sent", "lastReminder", now)
	}

	// Schedule next reconciliation
	return h.requeueAtNextEvent(ae, now)
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

// shouldRemind determines if a reminder should be sent based on the current time and last reminder
func (h *Handler) shouldRemind(ae *v1.ApprovalExpiration, now time.Time) bool {
	lastReminder := ae.Status.LastReminder

	expired := now.After(ae.Spec.Expiration.Time) || now.Equal(ae.Spec.Expiration.Time)

	if expired || now.After(ae.Spec.DailyReminder.Time) {
		// Daily: send if never reminded OR last reminder was >= 1 day ago
		return lastReminder == nil || now.After(lastReminder.Add(24*time.Hour))
	}

	if now.After(ae.Spec.WeeklyReminder.Time) {
		// Weekly: send if never reminded OR last reminder was >= 1 week ago
		return lastReminder == nil || now.After(lastReminder.Add(7*24*time.Hour))
	}

	// Too early
	return false
}

// ensureApprovalExpired transitions the parent Approval to EXPIRED state
func (h *Handler) ensureApprovalExpired(ctx context.Context, approval *v1.Approval) error {
	if approval.Spec.State == v1.ApprovalStateExpired {
		return nil // Already expired, no-op
	}

	// Add system Decision (matches auto-approval pattern)
	approval.Spec.Decisions = append(approval.Spec.Decisions, v1.Decision{
		Name:           "System",
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
func (h *Handler) sendReminder(ctx context.Context, ae *v1.ApprovalExpiration, approval *v1.Approval, now time.Time) error {
	// Determine reminder type
	var reminderType string
	var daysRemaining int64

	expired := now.After(ae.Spec.Expiration.Time) || now.Equal(ae.Spec.Expiration.Time)
	switch {
	case expired:
		reminderType = "expired"
		daysRemaining = 0
	case now.After(ae.Spec.DailyReminder.Time):
		reminderType = "daily"
		daysRemaining = int64(ae.Spec.Expiration.Time.Sub(now).Hours() / 24)
	default:
		reminderType = "weekly"
		daysRemaining = int64(ae.Spec.Expiration.Time.Sub(now).Hours() / 24)
	}

	// Build notification data with template properties
	notificationData := util.NotificationData{
		Owner:                  ae,
		SendToChannelNamespace: approval.Namespace,
		StateNew:               string(approval.Spec.State),
		StateOld:               string(approval.Spec.State), // State hasn't changed yet for reminders
		Target:                 &approval.Spec.Target,
		Requester:              &approval.Spec.Requester,
		Decider:                &approval.Spec.Decider,
		Scenario:               util.NotificationScenarioUpdated,
		Action:                 approval.Spec.Action,
		ExpirationDate:         ae.Spec.Expiration.Format("2006-01-02"),
		DaysRemaining:          fmt.Sprintf("%d", daysRemaining),
		ReminderType:           reminderType,
	}

	// Send to decider
	notificationData.Actor = util.ActorDecider
	deciderRef, err := util.SendReminderNotification(ctx, h.client, &notificationData)
	if err != nil {
		return errors.Wrap(err, "failed to send decider reminder")
	}

	// Send to requester
	notificationData.Actor = util.ActorRequester
	requesterRef, err := util.SendReminderNotification(ctx, h.client, &notificationData)
	if err != nil {
		return errors.Wrap(err, "failed to send requester reminder")
	}

	// Store last notification ref (we'll keep the requester one)
	ae.Status.LastNotificationRef = requesterRef

	_ = deciderRef // Used for sending, but we only store one ref

	return nil
}

// requeueAtNextEvent schedules the next reconciliation at the appropriate time
func (h *Handler) requeueAtNextEvent(ae *v1.ApprovalExpiration, now time.Time) error {
	var nextEvent time.Time

	switch {
	case now.Before(ae.Spec.WeeklyReminder.Time):
		// Not yet in reminder period
		nextEvent = ae.Spec.WeeklyReminder.Time

	case now.Before(ae.Spec.DailyReminder.Time):
		// In weekly reminder period
		if ae.Status.LastReminder != nil {
			nextWeekly := ae.Status.LastReminder.Add(7 * 24 * time.Hour)
			if nextWeekly.Before(ae.Spec.DailyReminder.Time) {
				nextEvent = nextWeekly
			} else {
				nextEvent = ae.Spec.DailyReminder.Time
			}
		} else {
			nextEvent = now // Should remind now
		}

	case now.Before(ae.Spec.Expiration.Time):
		// In daily reminder period
		if ae.Status.LastReminder != nil {
			nextDaily := ae.Status.LastReminder.Add(24 * time.Hour)
			if nextDaily.Before(ae.Spec.Expiration.Time) {
				nextEvent = nextDaily
			} else {
				nextEvent = ae.Spec.Expiration.Time
			}
		} else {
			nextEvent = now
		}

	default:
		// Already expired - requeue in 1 day for continued daily reminders
		if ae.Status.LastReminder != nil {
			nextEvent = ae.Status.LastReminder.Add(24 * time.Hour)
		} else {
			nextEvent = now
		}
	}

	delay := nextEvent.Sub(now)
	if delay <= 0 {
		delay = time.Second // Immediate retry
	}

	return ctrlerrors.RetryableWithDelayErrorf(delay, "next expiration event at %s", nextEvent.Format(time.RFC3339))
}
