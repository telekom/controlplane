// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalexpiration

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/handler/util"
	commonclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/reminder"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

var _ handler.Handler[*v1.ApprovalExpiration] = &Handler{}

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) CreateOrUpdate(ctx context.Context, ae *v1.ApprovalExpiration) error {
	logger := log.FromContext(ctx)

	c := commonclient.ClientFromContextOrDie(ctx)

	// Fetch the parent Approval
	approval := &v1.Approval{}
	if err := c.Get(ctx, client.ObjectKey{Name: ae.Spec.Approval.Name, Namespace: ae.Spec.Approval.Namespace}, approval); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Parent gone — GC via ownerRef will clean this up
			return nil
		}
		return errors.Wrap(err, "failed to fetch parent approval")
	}

	// Only process when approval is active (Granted or Suspended)
	if approval.Spec.State != v1.ApprovalStateGranted && approval.Spec.State != v1.ApprovalStateSuspended {
		logger.V(1).Info("Skipping expiration handler: approval is not in an active state", "state", approval.Spec.State)
		ae.SetCondition(condition.NewDoneProcessingCondition("ApprovalExpiration processed"))
		ae.SetCondition(
			condition.NewReadyCondition("Ready", "ApprovalExpiration processed successfully - parent approval is not active (Granted or Suspended)"))
		return nil
	}

	now := time.Now()

	if now.After(ae.Spec.Expiration.Time) || now.Equal(ae.Spec.Expiration.Time) {
		logger.Info("Approval has expired, transitioning to Expired state", "expiration", ae.Spec.Expiration.Time)
		_, err := c.CreateOrUpdate(ctx, approval, func() error {
			approval.Spec.State = v1.ApprovalStateExpired
			approval.Spec.Decisions = append(approval.Spec.Decisions, v1.Decision{
				Name:           v1.SystemDecisionName,
				Comment:        fmt.Sprintf("Approval expired after reaching expiration deadline %s", ae.Spec.Expiration.Format(time.RFC3339)),
				Timestamp:      &metav1.Time{Time: now},
				ResultingState: v1.ApprovalStateExpired,
			})
			return nil
		})
		if err != nil {
			return errors.Wrap(err, "failed to update approval state to Expired")
		}
		return nil
	}

	pending := reminder.Evaluate(ae.Spec.Expiration.Time, ae.Spec.Thresholds, ae.Status.SentReminders, now)
	if len(pending) > 0 {
		p := pending[0]
		notificationRef, err := h.sendReminder(ctx, ae, approval, now)
		if err != nil {
			return errors.Wrap(err, "failed to send reminder")
		}

		ae.Status.SentReminders = reminder.UpsertSent(ae.Status.SentReminders, &reminder.SentReminder{
			Threshold: p.Key,
			Ref:       *notificationRef,
			SentAt:    metav1.NewTime(now),
		})
	}

	delay := reminder.NextRequeue(ae.Spec.Expiration.Time, ae.Spec.Thresholds, ae.Status.SentReminders, now)
	if delay <= 0 {
		delay = time.Second
	}
	contextutil.SetRequeueAfter(ctx, delay)

	ae.SetCondition(condition.NewDoneProcessingCondition("ApprovalExpiration processed"))
	ae.SetCondition(
		condition.NewReadyCondition("Ready", "ApprovalExpiration processed successfully"))

	return nil
}

func (h *Handler) Delete(ctx context.Context, ae *v1.ApprovalExpiration) error {
	return nil
}

func (h *Handler) sendReminder(ctx context.Context, ae *v1.ApprovalExpiration, approval *v1.Approval, now time.Time) (*commontypes.ObjectRef, error) {
	daysRemaining := int64(ae.Spec.Expiration.Time.Sub(now).Hours() / 24)
	if daysRemaining < 0 {
		daysRemaining = 0
	}

	notificationData := util.ReminderNotificationData{
		NotificationData: util.NotificationData{
			Owner:     ae,
			StateNew:  string(approval.Spec.State),
			StateOld:  string(approval.Spec.State),
			Target:    &approval.Spec.Target,
			Requester: &approval.Spec.Requester,
			Decider:   &approval.Spec.Decider,
			Scenario:  util.NotificationScenarioUpdated,
			Action:    approval.Spec.Action,
		},
		ExpirationDate: ae.Spec.Expiration.Format(time.RFC3339),
		DaysRemaining:  fmt.Sprintf("%d", daysRemaining),
		IsExpired:      daysRemaining == 0,
	}

	// Send to decider
	notificationData.Actor = util.ActorDecider
	notificationData.SendToChannelNamespace = approval.Spec.Decider.ApplicationRef.Namespace
	if _, err := util.SendReminderNotification(ctx, &notificationData); err != nil {
		return nil, errors.Wrap(err, "failed to send decider reminder")
	}

	// Send to requester — store this ref in SentReminders
	notificationData.Actor = util.ActorRequester
	notificationData.SendToChannelNamespace = approval.Spec.Requester.ApplicationRef.Namespace
	requesterRef, err := util.SendReminderNotification(ctx, &notificationData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send requester reminder")
	}

	return requesterRef, nil
}
