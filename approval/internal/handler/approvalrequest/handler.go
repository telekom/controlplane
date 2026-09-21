// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	approval_condition "github.com/telekom/controlplane/approval/internal/condition"
	"github.com/telekom/controlplane/approval/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

var _ handler.Handler[*approvalv1.ApprovalRequest] = &ApprovalRequestHandler{}

type ApprovalRequestHandler struct {
	// Reader is an uncached API reader for source-liveness checks during grant
	// materialization. Using the manager's cached client here would risk reading
	// stale data when a source request has been deleted and recreated between
	// cache syncs.
	Reader ctrlclient.Reader
}

func (h *ApprovalRequestHandler) CreateOrUpdate(ctx context.Context, approvalReq *approvalv1.ApprovalRequest) error {
	logger := log.FromContext(ctx)

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
		logger.Info("ApprovalRequest is auto approved")
		if approvalReq.Spec.State != approvalv1.ApprovalStateGranted { // TODO: move this to validation webhook
			approvalReq.SetCondition(condition.NewBlockedCondition("Request is auto approved and should be granted"))
			return nil
		}

		err := h.handleGranted(ctx, approvalReq)
		if err != nil {
			return errors.Wrap(err, "failed to handle granted approval")
		}
		return nil
	}

	switch approvalReq.Spec.State {

	case approvalv1.ApprovalStateGranted:
		logger.Info("ApprovalRequest has been approved")
		err := h.handleGranted(ctx, approvalReq)
		if err != nil {
			return errors.Wrap(err, "failed to handle granted approval")
		}

	case approvalv1.ApprovalStateSemigranted:
		logger.Info("ApprovalRequest has been partially approved")
		approvalReq.SetCondition(approval_condition.NewSemigrantedCondition())
		approvalReq.SetCondition(condition.NewProcessingCondition("Semigranted", "Request partially approved, awaiting second approval"))
		approvalReq.SetCondition(condition.NewNotReadyCondition("Semigranted", "Request has been partially approved"))

	case approvalv1.ApprovalStateRejected:
		logger.Info("ApprovalRequest has been rejected")
		approvalReq.SetCondition(approval_condition.NewRejectedCondition())
		approvalReq.SetCondition(condition.NewDoneProcessingCondition("Request rejected"))
		approvalReq.SetCondition(condition.NewNotReadyCondition("Rejected", "Request has been rejected"))

	case approvalv1.ApprovalStatePending:
		logger.Info("ApprovalRequest is still pending")
		approvalReq.SetCondition(approval_condition.NewPendingCondition())
		approvalReq.SetCondition(condition.NewProcessingCondition(builder.ReasonApprovalPending, "Request is pending"))
		approvalReq.SetCondition(condition.NewNotReadyCondition("Pending", "Request is pending"))

	default:
		logger.Info("ApprovalRequest is in an unknown state")
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
	// Semigranted is an intermediate state; only deciders need to know
	if approvalRequest.Spec.State == approvalv1.ApprovalStateSemigranted {
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
	if approvalReq.GetGeneration() == 1 {
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
		Action:                 approvalReq.Spec.Action,
		ApprovalKey:            approvalReq.Spec.ApprovalKey,
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
			Action:                 approvalReq.Spec.Action,
			ApprovalKey:            approvalReq.Spec.ApprovalKey,
		})
		if err != nil {
			return errors.Wrapf(err, "Failed to send notification to requester %q while handling approval request %+v", approvalReq.Spec.Requester.TeamName, approvalReq)
		}
		approvalReq.Status.NotificationRefs = append(approvalReq.Status.NotificationRefs, *notificationRef)
	}

	return nil
}

func (h *ApprovalRequestHandler) handleGranted(ctx context.Context, approvalReq *approvalv1.ApprovalRequest) error {
	logger := log.FromContext(ctx)
	c := client.ClientFromContextOrDie(ctx)

	approvalObj, err := newApprovalFromApprovalRequest(approvalReq)
	if err != nil {
		return errors.Wrap(err, "failed to derive approval name")
	}

	isKeyed := approvalReq.Spec.ApprovalKey != ""

	mutate := func() error {
		// For scoped Approvals that already exist, validate identity and protect revocations.
		if isKeyed && approvalObj.UID != "" {
			if approvalObj.Spec.ApprovalKey != approvalReq.Spec.ApprovalKey {
				return fmt.Errorf("scoped approval %s: approvalKey mismatch: existing %q != request %q",
					approvalObj.Name, approvalObj.Spec.ApprovalKey, approvalReq.Spec.ApprovalKey)
			}
			if !approvalv1.ScopedIdentityMatch(approvalObj.Spec.Target, approvalReq.Spec.Target,
				approvalObj.Spec.ApprovalKey, approvalReq.Spec.ApprovalKey) {
				return fmt.Errorf("scoped approval %s: target identity mismatch", approvalObj.Name)
			}

			// Preserve active revocations: do not overwrite Rejected/Suspended with a new grant.
			isRevoked := approvalObj.Spec.State == approvalv1.ApprovalStateRejected ||
				approvalObj.Spec.State == approvalv1.ApprovalStateSuspended
			isExpiredFromSuspended := approvalObj.Spec.State == approvalv1.ApprovalStateExpired &&
				approvalObj.Status.LastState == approvalv1.ApprovalStateSuspended
			if isRevoked || isExpiredFromSuspended {
				logger.Info("Scoped approval is revoked; preserving revocation",
					"state", approvalObj.Spec.State)
				return nil
			}
		}

		// Determine the authoritative source for building the Approval spec.
		// Scoped requests load a validated fresh copy from the API server (which
		// also enforces the ambiguity guard) BEFORE the idempotency check so that
		// an already-bound request cannot bypass the guard.
		// Unscoped requests fall through to the legacy idempotency + liveness path.
		source := approvalReq
		if isKeyed {
			freshSource, err := h.loadSoleLiveScopedGrantSource(ctx, approvalReq)
			if err != nil {
				return fmt.Errorf("scoped grant-source selection: %w", err)
			}
			source = freshSource

			// Scoped idempotency: check using the validated source's identity.
			if approvalObj.Spec.ApprovedRequest != nil &&
				approvalObj.Spec.ApprovedRequest.Name == source.Name &&
				approvalObj.Spec.ApprovedRequest.Namespace == source.Namespace &&
				approvalObj.Spec.ApprovedRequest.UID == source.UID {
				approvalv1.SetApprovalKeyLabel(approvalObj, source.Spec.ApprovalKey)
				logger.Info("Approval has already been processed for this request")
				return nil
			}
		} else {
			// Legacy (unscoped) idempotency check.
			if approvalObj.Spec.ApprovedRequest != nil && approvalObj.Spec.ApprovedRequest.Name == approvalReq.Name {
				logger.Info("Approval has already been processed for this request")
				return nil
			}

			// Legacy (unscoped): re-read via uncached reader to verify liveness.
			freshAR := &approvalv1.ApprovalRequest{}
			if err := h.Reader.Get(ctx, ctrlclient.ObjectKeyFromObject(approvalReq), freshAR); err != nil {
				return fmt.Errorf("re-reading source request: %w", err)
			}
			if freshAR.UID != approvalReq.UID {
				return fmt.Errorf("source request was recreated (expected UID %s, got %s)", approvalReq.UID, freshAR.UID)
			}
			if freshAR.DeletionTimestamp != nil {
				return fmt.Errorf("source request is terminating")
			}
			if freshAR.Spec.State != approvalv1.ApprovalStateGranted {
				return fmt.Errorf("source request is no longer granted (state: %s)", freshAR.Spec.State)
			}
		}

		// Set controller owner reference only when creating (no existing refs).
		if len(approvalObj.GetOwnerReferences()) == 0 {
			setControllerReferenceForRef(approvalObj, &source.Spec.Target)
		}

		approvalObj.Spec = approvalv1.ApprovalSpec{
			Strategy: source.Spec.Strategy,
			State:    approvalv1.ApprovalStateGranted,

			Requester: source.Spec.Requester,
			Decider:   source.Spec.Decider,
			Target:    source.Spec.Target,
			Action:    source.Spec.Action,
			Decisions: source.Spec.Decisions,

			ApprovedRequest: types.ObjectRefFromObject(source),
			ApprovalKey:     source.Spec.ApprovalKey,
		}

		approvalv1.SetApprovalLabels(approvalObj, source.Spec.Target,
			source.Spec.Requester.TeamName,
			source.Spec.Decider.TeamName,
			source.Spec.Action,
			string(source.Spec.Strategy))
		approvalv1.SetApprovalKeyLabel(approvalObj, source.Spec.ApprovalKey)

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, approvalObj, mutate)
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

func setControllerReferenceForRef(obj types.Object, objRef *types.TypedObjectRef) {
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

func newApprovalFromApprovalRequest(approvalReq *approvalv1.ApprovalRequest) (*approvalv1.Approval, error) {
	name := approvalv1.ApprovalName(approvalReq.Spec.Target.Kind, approvalReq.Spec.Target.Name)

	if approvalReq.Spec.ApprovalKey != "" {
		scopedName, err := approvalv1.ScopedApprovalName(approvalReq.Spec.Target, approvalReq.Spec.ApprovalKey)
		if err != nil {
			return nil, fmt.Errorf("computing scoped approval name: %w", err)
		}
		name = scopedName
	}

	return &approvalv1.Approval{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: approvalReq.Namespace,
		},
		Spec: approvalv1.ApprovalSpec{},
	}, nil
}
