// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"github.com/pkg/errors"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	approval_condition "github.com/telekom/controlplane/approval/internal/condition"
	"github.com/telekom/controlplane/approval/internal/handler/util"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ handler.Handler[*approvalv1.ApprovalRequest] = &ApprovalRequestHandler{}

type ApprovalRequestHandler struct {
}

func (h *ApprovalRequestHandler) CreateOrUpdate(ctx context.Context, approvalReq *approvalv1.ApprovalRequest) error {
	log := log.FromContext(ctx)

	// handle the notifications first
	err := handleNotifications(ctx, approvalReq)
	if err != nil {
		// todo - decide if we want to fail here, or a failed notification is acceptable
		return errors.Wrapf(err, "Failed to send notification about approval request %+v", approvalReq)
	}

	fsm := ApprovalStrategyFSM[approvalReq.Spec.Strategy]
	approvalReq.Status.AvailableTransitions = fsm.AvailableTransitions(approvalReq.Spec.State)
	approvalReq.Status.LastState = approvalReq.Spec.State

	if approvalReq.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
		log.Info("ApprovalRequest is auto approved")
		if approvalReq.Spec.State != approvalv1.ApprovalStateGranted { // TODO: move this to validation webhook
			approvalReq.SetCondition(condition.NewBlockedCondition("Request is auto approved and should be granted"))
			return nil
		}

		err := handleGranted(ctx, approvalReq)
		if err != nil {
			return errors.Wrap(err, "failed to handle granted approval")
		}
		return nil
	}

	switch approvalReq.Spec.State {

	case approvalv1.ApprovalStateGranted:
		log.Info("ApprovalRequest has been approved")
		err := handleGranted(ctx, approvalReq)
		if err != nil {
			return errors.Wrap(err, "failed to handle granted approval")
		}

	case approvalv1.ApprovalStateRejected:
		log.Info("ApprovalRequest has been rejected")
		approvalReq.SetCondition(approval_condition.NewRejectedCondition())
		approvalReq.SetCondition(condition.NewDoneProcessingCondition("Request rejected"))
		approvalReq.SetCondition(condition.NewNotReadyCondition("Rejected", "Request has been rejected"))

	case approvalv1.ApprovalStatePending:
		log.Info("ApprovalRequest is still pending")
		approvalReq.SetCondition(approval_condition.NewPendingCondition())
		approvalReq.SetCondition(condition.NewProcessingCondition("ApprovalPending", "Request is pending"))
		approvalReq.SetCondition(condition.NewNotReadyCondition("Pending", "Request is pending"))

	default:
		log.Info("ApprovalRequest is in an unknown state")
		approvalReq.SetCondition(condition.NewBlockedCondition("Request is in an unknown state"))
		approvalReq.SetCondition(condition.NewNotReadyCondition("Invalid", "Request is in an unknown state"))
	}

	return nil
}

func (h *ApprovalRequestHandler) Delete(ctx context.Context, approvalReq *approvalv1.ApprovalRequest) error {
	return nil
}

func shouldNotifyRequester(approvalRequest *approvalv1.ApprovalRequest) bool {
	// currently only the decider is notified about this
	if approvalRequest.Spec.State == approvalv1.ApprovalStatePending {
		return false
	}

	return true
}

func handleNotifications(ctx context.Context, approvalReq *approvalv1.ApprovalRequest) error {
	// no change in status - nothing to notify about
	if approvalReq.Spec.State == approvalReq.Status.LastState {
		return nil
	}

	contextutil.RecorderFromContextOrDie(ctx).Eventf(approvalReq,
		"Normal", "Notification", "State changed from %s to %s", approvalReq.Status.LastState, approvalReq.Spec.State,
	)

	var scenario util.NotificationScenario
	if approvalReq.ObjectMeta.GetGeneration() == 1 {
		scenario = util.NotificationScenarioCreated
	} else {
		scenario = util.NotificationScenarioUpdated
	}

	// always notify the Decider
	notificationRef, err := util.SendNotification(ctx, &util.NotificationData{
		Owner:                  approvalReq,
		SendToChannelNamespace: approvalReq.Spec.Decider.ApplicationRef.Namespace,
		StateNew:               string(approvalReq.Spec.State),
		Target:                 &approvalReq.Spec.Target,
		Requester:              &approvalReq.Spec.Requester,
		Decider:                &approvalReq.Spec.Decider,
		Scenario:               scenario,
		Actor:                  util.ActorDecider,
	})

	if err != nil {
		return errors.Wrapf(err, "Failed to send notification to decider %q while handling approval request %+v", approvalReq.Spec.Decider.TeamName, approvalReq)
	}
	approvalReq.Status.NotificationRefs = append(approvalReq.Status.NotificationRefs, *notificationRef)

	// if relevant notify the requester
	if shouldNotifyRequester(approvalReq) {
		notificationRef, err := util.SendNotification(ctx, &util.NotificationData{
			Owner:                  approvalReq,
			SendToChannelNamespace: approvalReq.Spec.Requester.ApplicationRef.Namespace,
			StateNew:               string(approvalReq.Spec.State),
			Target:                 &approvalReq.Spec.Target,
			Requester:              &approvalReq.Spec.Requester,
			Decider:                &approvalReq.Spec.Decider,
			Scenario:               scenario,
			Actor:                  util.ActorRequester,
		})
		if err != nil {
			return errors.Wrapf(err, "Failed to send notification to requester %q while handling approval request %+v", approvalReq.Spec.Requester.TeamName, approvalReq)
		}
		approvalReq.Status.NotificationRefs = append(approvalReq.Status.NotificationRefs, *notificationRef)
	}

	return nil
}

func handleGranted(ctx context.Context, approvalReq *approvalv1.ApprovalRequest) error {
	log := log.FromContext(ctx)
	c := client.ClientFromContextOrDie(ctx)

	approvalObj := newApprovalFromApprovalRequest(approvalReq)

	mutate := func() error {
		if approvalObj.Spec.ApprovedRequest != nil && approvalObj.Spec.ApprovedRequest.Name == approvalReq.Name {
			log.Info("Approval has already been processed for this request")
			return nil
		}

		setControllerReferenceForRef(approvalObj, approvalReq.Spec.Target)

		approvalObj.Spec = approvalv1.ApprovalSpec{
			Strategy: approvalReq.Spec.Strategy,
			State:    approvalv1.ApprovalStateGranted,

			Requester: approvalReq.Spec.Requester,
			Decider:   approvalReq.Spec.Decider,
			Target:    approvalReq.Spec.Target,
			Action:    approvalReq.Spec.Action,
			Decisions: approvalReq.Spec.Decisions,

			ApprovedRequest: types.ObjectRefFromObject(approvalReq),
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, approvalObj, mutate)
	if err != nil {
		return errors.Wrap(err, "failed to create or update approval")
	}

	approvalReq.Status.Approval = *types.ObjectRefFromObject(approvalObj)

	approvalReq.SetCondition(approval_condition.NewApprovedCondition())
	approvalReq.SetCondition(condition.NewDoneProcessingCondition("Request has been approved"))
	approvalReq.SetCondition(
		condition.NewReadyCondition("Granted", "Request has been approved and approval is granted"))

	return nil
}

func setControllerReferenceForRef(obj types.Object, objRef types.TypedObjectRef) {
	gvk := objRef.GroupVersionKind()
	ref := metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               objRef.Name,
		UID:                objRef.UID,
		BlockOwnerDeletion: ptr.To(true),
		Controller:         ptr.To(true),
	}

	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), ref))
}

func newApprovalFromApprovalRequest(approvalReq *approvalv1.ApprovalRequest) *approvalv1.Approval {
	return &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      approvalv1.ApprovalName(approvalReq.Spec.Target.Kind, approvalReq.Spec.Target.Name),
			Namespace: approvalReq.Namespace,
		},
		Spec: approvalv1.ApprovalSpec{},
	}
}
