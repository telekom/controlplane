// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"fmt"
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
	cfg *config.ExpirationConfig
}

// NewHandler creates a new ApprovalHandler with the provided configuration
func NewHandler(client client.Client, cfg *config.ExpirationConfig) *ApprovalHandler {
	return &ApprovalHandler{
		cfg: cfg,
	}
}

func (h *ApprovalHandler) CreateOrUpdate(ctx context.Context, approval *approvalv1.Approval) error {
	// handle the notifications first
	err := handleNotifications(ctx, approval)
	if err != nil {
		// todo - decide if we want to fail here, or a failed notification is acceptable
		return errors.Wrapf(err, "Failed to send notification about approval %+v", approval)
	}

	fsm := ApprovalStrategyFSM[approval.Spec.Strategy]
	// Filter out system-only actions (Expire) from available transitions
	approval.Status.AvailableTransitions = filterSystemActions(fsm.AvailableTransitions(approval.Spec.State))

	// Capture state change BEFORE updating LastState (needed for expiration logic)
	stateChanged := approval.Spec.State != approval.Status.LastState
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
		approval.SetCondition(approval_condition.NewExpiredCondition())
		approval.SetCondition(condition.NewProcessingCondition("Expired", "Approval has expired"))
		approval.SetCondition(condition.NewReadyCondition("Expired", "Approval has expired but can be re-approved"))

	}

	// Handle ApprovalExpiration lifecycle (do this after conditions are set)
	if err := h.handleExpiration(ctx, approval, stateChanged); err != nil {
		return errors.Wrap(err, "failed to handle expiration")
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

// filterSystemActions removes system-only actions (Expire) from the available transitions
func filterSystemActions(transitions approvalv1.AvailableTransitions) approvalv1.AvailableTransitions {
	var filtered approvalv1.AvailableTransitions
	for _, t := range transitions {
		if t.Action != approvalv1.ApprovalActionExpire {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// handleExpiration manages the lifecycle of ApprovalExpiration resources
func (h *ApprovalHandler) handleExpiration(ctx context.Context, approval *approvalv1.Approval, stateChanged bool) error {
	// Only for non-Auto strategies
	if approval.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
		return nil
	}

	c := commonclient.ClientFromContextOrDie(ctx)

	switch approval.Spec.State {
	case approvalv1.ApprovalStateGranted:
		if stateChanged {
			// Create or update ApprovalExpiration with fresh dates
			// (covers initial GRANTED and REALLOW from EXPIRED)
			return createOrUpdateApprovalExpiration(ctx, c, approval, h.cfg)
		}
		// State unchanged, leave ApprovalExpiration alone

	case approvalv1.ApprovalStateSuspended:
		// Clock keeps ticking, leave ApprovalExpiration alone

	case approvalv1.ApprovalStateExpired:
		// Already expired, leave ApprovalExpiration alone

	case approvalv1.ApprovalStateRejected:
		if stateChanged {
			// Delete ApprovalExpiration
			return deleteApprovalExpiration(ctx, c, approval)
		}

	case approvalv1.ApprovalStatePending, approvalv1.ApprovalStateSemigranted:
		// No ApprovalExpiration should exist in these states
		// (safety: delete if exists, but shouldn't happen)
		return deleteApprovalExpiration(ctx, c, approval)
	}

	return nil
}

// createOrUpdateApprovalExpiration creates or updates an ApprovalExpiration with fresh dates
func createOrUpdateApprovalExpiration(ctx context.Context, c commonclient.JanitorClient, approval *approvalv1.Approval, cfg *config.ExpirationConfig) error {
	now := time.Now()
	expirationDate := now.Add(cfg.ExpirationDuration)

	ae := &approvalv1.ApprovalExpiration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s--expiration", approval.Name),
			Namespace: approval.Namespace,
		},
	}

	mutate := func() error {
		if err := controllerutil.SetControllerReference(approval, ae, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}
		ae.Spec = approvalv1.ApprovalExpirationSpec{
			Approval: types.ObjectRef{
				Name:      approval.Name,
				Namespace: approval.Namespace,
			},
			Expiration: metav1.Time{Time: expirationDate},
			Thresholds: cfg.DefaultThresholds,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, ae, mutate)
	if err != nil {
		return errors.Wrap(err, "failed to create or update ApprovalExpiration")
	}

	log.FromContext(ctx).Info("Created or updated ApprovalExpiration", "name", ae.Name, "expiration", expirationDate)
	return nil
}

// deleteApprovalExpiration deletes the ApprovalExpiration if it exists
func deleteApprovalExpiration(ctx context.Context, c commonclient.JanitorClient, approval *approvalv1.Approval) error {
	ae := &approvalv1.ApprovalExpiration{}
	key := client.ObjectKey{
		Name:      fmt.Sprintf("%s--expiration", approval.Name),
		Namespace: approval.Namespace,
	}

	err := c.Get(ctx, key, ae)
	if err != nil {
		// Doesn't exist, nothing to delete
		return client.IgnoreNotFound(err)
	}

	// Exists, delete it
	if err := c.Delete(ctx, ae); err != nil {
		return errors.Wrap(err, "failed to delete ApprovalExpiration")
	}

	log.FromContext(ctx).Info("Deleted ApprovalExpiration", "name", ae.Name)
	return nil
}
