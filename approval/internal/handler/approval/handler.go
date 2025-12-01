// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/approval/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	approval_condition "github.com/telekom/controlplane/approval/internal/condition"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*approvalv1.Approval] = &ApprovalHandler{}

type ApprovalHandler struct {
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

	case approvalv1.ApprovalStateSuspended:
		approval.SetCondition(approval_condition.NewSuspendedCondition())
		approval.SetCondition(condition.NewProcessingCondition("Suspended", "Approval is suspended"))
		approval.SetCondition(condition.NewReadyCondition("Suspended", "Approval is suspended"))

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
		})

		if err != nil {
			return errors.Wrapf(err, "Failed to send notification to decider %q while handling approval %+v", approval.Spec.Decider.TeamName, approval)
		}
		approval.Status.NotificationRefs = append(approval.Status.NotificationRefs, *notificationRef)

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
		})

		if err != nil {
			return errors.Wrapf(err, "Failed to send notification to requester %q while handling approval %+v", approval.Spec.Requester.TeamName, approval)
		}
		approval.Status.NotificationRefs = append(approval.Status.NotificationRefs, *notificationRef)
	}

	return nil
}

func (h *ApprovalHandler) Delete(ctx context.Context, approval *approvalv1.Approval) error {
	log := log.FromContext(ctx)

	log.Info("Approval deleted")
	return nil
}
