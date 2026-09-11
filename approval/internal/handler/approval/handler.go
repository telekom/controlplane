// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	approval_condition "github.com/telekom/controlplane/approval/internal/condition"
	"github.com/telekom/controlplane/approval/internal/config"
	"github.com/telekom/controlplane/approval/internal/handler/util"
	commonclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

var _ handler.Handler[*approvalv1.Approval] = &ApprovalHandler{}

type ApprovalHandler struct {
	expirationCfg *config.ExpirationConfig
}

func NewHandler(cfg *config.ExpirationConfig) *ApprovalHandler {
	return &ApprovalHandler{expirationCfg: cfg}
}

func (h *ApprovalHandler) CreateOrUpdate(ctx context.Context, approval *approvalv1.Approval) error {
	// handle the notifications first
	err := handleNotifications(ctx, approval)
	if err != nil {
		// todo - decide if we want to fail here, or a failed notification is acceptable
		return errors.Wrapf(err, "Failed to send notification about approval %+v", approval)
	}

	fsm := ApprovalStrategyFSM[approval.Spec.Strategy]
	approval.Status.AvailableTransitions = fsm.AvailableTransitions(approval.Spec.State)

	// Capture state change BEFORE updating LastState (needed for expiration logic)
	previousState := approval.Status.LastState
	stateChanged := approval.Spec.State != previousState
	approval.Status.LastState = approval.Spec.State

	switch approval.Spec.State {
	case approvalv1.ApprovalStateGranted:
		approval.SetCondition(approval_condition.NewApprovedCondition())
		approval.SetCondition(condition.NewDoneProcessingCondition("Approval granted"))
		approval.SetCondition(condition.NewReadyCondition("Approved", "Approval has been granted"))

	case approvalv1.ApprovalStatePending:
		approval.SetCondition(approval_condition.NewPendingCondition())
		approval.SetCondition(condition.NewBlockedCondition("Approval is pending"))
		approval.SetCondition(condition.NewNotReadyCondition("Pending", "Approval is pending"))

	case approvalv1.ApprovalStateRejected:
		approval.SetCondition(approval_condition.NewRejectedCondition())
		approval.SetCondition(condition.NewDoneProcessingCondition("Approval rejected"))
		approval.SetCondition(condition.NewNotReadyCondition("Rejected", "Approval has been rejected"))

	case approvalv1.ApprovalStateSemigranted:
		approval.SetCondition(approval_condition.NewSemigrantedCondition())
		approval.SetCondition(condition.NewProcessingCondition("Semigranted", "Approval partially granted, awaiting second approval"))
		approval.SetCondition(condition.NewNotReadyCondition("Semigranted", "Approval has been partially granted"))

	case approvalv1.ApprovalStateSuspended:
		approval.SetCondition(approval_condition.NewSuspendedCondition())
		approval.SetCondition(condition.NewProcessingCondition("Suspended", "Approval is suspended"))
		approval.SetCondition(condition.NewReadyCondition("Suspended", "Approval is suspended"))

	case approvalv1.ApprovalStateExpired:
		approval.SetCondition(metav1.Condition{
			Type:    "Approved",
			Status:  metav1.ConditionFalse,
			Reason:  "Expired",
			Message: "Request has expired",
		})
		approval.SetCondition(condition.NewDoneProcessingCondition("Approval expired"))
		approval.SetCondition(condition.NewNotReadyCondition("Expired", "Approval has expired"))

	}

	if err := h.handleExpiration(ctx, approval, stateChanged, previousState); err != nil {
		return errors.Wrap(err, "failed to handle expiration")
	}

	return nil
}

// handleExpiration manages the ApprovalExpiration child CR and the ExpiresAt status field.
func (h *ApprovalHandler) handleExpiration(ctx context.Context, approval *approvalv1.Approval, stateChanged bool, previousState approvalv1.ApprovalState) error {
	c := commonclient.ClientFromContextOrDie(ctx)

	switch approval.Spec.State {
	case approvalv1.ApprovalStateGranted:
		// Restart the expiry clock only on a real transition into Granted,
		// and not when resuming from Suspended (clock kept ticking during suspension).
		if stateChanged && previousState != approvalv1.ApprovalStateSuspended {
			return h.createOrUpdateApprovalExpiration(ctx, c, approval)
		}
		// Steady-state Granted or resume from Suspended: leave expiry CR and ExpiresAt untouched.

	case approvalv1.ApprovalStateSuspended:
		// Clock keeps ticking, leave expiry CR and ExpiresAt untouched.

	case approvalv1.ApprovalStateRejected:
		if stateChanged {
			approval.Status.ExpiresAt = nil
			return deleteApprovalExpiration(ctx, c, approval)
		}

	case approvalv1.ApprovalStatePending, approvalv1.ApprovalStateSemigranted:
		// Defensive: no expiry CR should exist in these states.
		approval.Status.ExpiresAt = nil
		return deleteApprovalExpiration(ctx, c, approval)
	}

	return nil
}

func (h *ApprovalHandler) createOrUpdateApprovalExpiration(ctx context.Context, c commonclient.JanitorClient, approval *approvalv1.Approval) error {
	expirationTime := time.Now().Add(h.expirationCfg.ExpirationDuration)

	ae := &approvalv1.ApprovalExpiration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      approval.Name,
			Namespace: approval.Namespace,
		},
	}

	_, err := c.CreateOrUpdate(ctx, ae, func() error {
		if err := controllerutil.SetControllerReference(approval, ae, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}
		ae.Labels = approval.GetLabels()
		ae.Spec = approvalv1.ApprovalExpirationSpec{
			Approval: types.ObjectRef{
				Name:      approval.Name,
				Namespace: approval.Namespace,
			},
			Expiration: metav1.NewTime(expirationTime),
			Thresholds: h.expirationCfg.DefaultThresholds,
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to create or update ApprovalExpiration")
	}

	// Mirror the expiration time to the approval status so consumers don't need to look up the child CR.
	approval.Status.ExpiresAt = &metav1.Time{Time: expirationTime}
	return nil
}

func deleteApprovalExpiration(ctx context.Context, c commonclient.JanitorClient, approval *approvalv1.Approval) error {
	ae := &approvalv1.ApprovalExpiration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      approval.Name,
			Namespace: approval.Namespace,
		},
	}
	if err := c.Delete(ctx, ae); err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			return errors.Wrap(err, "failed to delete ApprovalExpiration")
		}
	}
	return nil
}

func handleNotifications(ctx context.Context, approval *approvalv1.Approval) error {
	// initial notification (approvalRequest granted) is handled by the approval request handler

	if (approval.Spec.State != approval.Status.LastState) && approval.Status.LastState != "" {
		contextutil.RecorderFromContextOrDie(ctx).Eventf(approval,
			"Normal", "Notification", "State changed from %s to %s", approval.Status.LastState, approval.Spec.State,
		)

		scenario := util.NotificationScenarioUpdated

		// notify the decider
		notificationRef, err := util.SendNotification(ctx, &util.NotificationData{
			Owner:                  approval,
			SendToChannelNamespace: approval.Spec.Decider.ApplicationRef.Namespace,
			StateNew:               string(approval.Spec.State),
			StateOld:               string(approval.Status.LastState),
			Target:                 &approval.Spec.Target,
			Requester:              &approval.Spec.Requester,
			Decider:                &approval.Spec.Decider,
			Scenario:               scenario,
			Actor:                  util.ActorDecider,
			Action:                 approval.Spec.Action,
		})
		if err != nil {
			return errors.Wrapf(err, "Failed to send notification to decider %q while handling approval %+v", approval.Spec.Decider.TeamName, approval)
		}
		approval.Status.NotificationRefs = append(approval.Status.NotificationRefs, *notificationRef)

		// Semigranted is an intermediate state; only deciders need to know.
		// Do not notify the requester until the approval reaches a final state.
		if approval.Spec.State != approvalv1.ApprovalStateSemigranted {
			// notify the requester
			notificationRef, err = util.SendNotification(ctx, &util.NotificationData{
				Owner:                  approval,
				SendToChannelNamespace: approval.Spec.Requester.ApplicationRef.Namespace,
				StateNew:               string(approval.Spec.State),
				StateOld:               string(approval.Status.LastState),
				Target:                 &approval.Spec.Target,
				Requester:              &approval.Spec.Requester,
				Decider:                &approval.Spec.Decider,
				Scenario:               scenario,
				Actor:                  util.ActorRequester,
				Action:                 approval.Spec.Action,
			})
			if err != nil {
				return errors.Wrapf(err, "Failed to send notification to requester %q while handling approval %+v", approval.Spec.Requester.TeamName, approval)
			}
			approval.Status.NotificationRefs = append(approval.Status.NotificationRefs, *notificationRef)
		}
	}

	return nil
}

func (h *ApprovalHandler) Delete(ctx context.Context, approval *approvalv1.Approval) error {
	logger := log.FromContext(ctx)
	logger.Info("Approval deleted")
	return nil
}
