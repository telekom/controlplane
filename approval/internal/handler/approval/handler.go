// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	approval_condition "github.com/telekom/controlplane/approval/internal/condition"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/api/v1/builder"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*approvalv1.Approval] = &ApprovalHandler{}

type ApprovalHandler struct {
}

func (h *ApprovalHandler) CreateOrUpdate(ctx context.Context, approval *approvalv1.Approval) error {

	if approval.Spec.State != approval.Status.LastState {
		contextutil.RecorderFromContextOrDie(ctx).Eventf(approval,
			"Normal", "Notification", "State changed from %s to %s", approval.Status.LastState, approval.Spec.State,
		)
		notify(ctx, approval)
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
		approval.SetCondition(condition.NewProcessingCondition("ApprovalPending", "Approval is pending"))
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

func notify(ctx context.Context, owner *approvalv1.Approval) {
	notificationBuilder := builder.New().
		WithOwner(owner).
		WithSender(notificationv1.SenderTypeSystem, "OrganizationService").
		WithDefaultChannels(ctx, owner.GetNamespace()).
		WithPurpose("approval").
		WithProperties(map[string]any{
			"env":   contextutil.EnvFromContextOrDie(ctx),
			"file":  owner.Spec.Resource.Name,
			"state": owner.Spec.State,
		})

	notification, err := notificationBuilder.Send(ctx)
	if err != nil {
		return
	}
	owner.Status.NotificationRef = types.ObjectRefFromObject(notification)
}

func (h *ApprovalHandler) Delete(ctx context.Context, approval *approvalv1.Approval) error {
	log := log.FromContext(ctx)

	log.Info("Approval deleted")
	return nil
}
