// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	stderrors "errors"
	"fmt"
	"maps"
	"slices"

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
		return errors.Wrapf(err, "Failed to send notification about approval request %q in namespace %q", approvalReq.Name, approvalReq.Namespace)
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
	// Normalise first: the list is a map-typed list keyed on namespace and name,
	// so objects persisted with duplicates must be healed before the next status write.
	approvalReq.Status.NotificationRefs = util.DedupRefs(approvalReq.Status.NotificationRefs)

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
		return errors.Wrapf(err, "Failed to send notification to decider %q while handling approval request %q", approvalReq.Spec.Decider.TeamName, approvalReq.Name)
	}
	approvalReq.Status.NotificationRefs = util.AppendUniqueRef(approvalReq.Status.NotificationRefs, *notificationRef)

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
			return errors.Wrapf(err, "Failed to send notification to requester %q while handling approval request %q", approvalReq.Spec.Requester.TeamName, approvalReq.Name)
		}
		approvalReq.Status.NotificationRefs = util.AppendUniqueRef(approvalReq.Status.NotificationRefs, *notificationRef)
	}

	return nil
}

func (h *ApprovalRequestHandler) handleGranted(ctx context.Context, approvalReq *approvalv1.ApprovalRequest) error {
	c := client.ClientFromContextOrDie(ctx)

	approvalObj, err := newApprovalFromApprovalRequest(approvalReq)
	if err != nil {
		return errors.Wrap(err, "failed to derive approval name")
	}

	isKeyed := approvalReq.Spec.ApprovalKey != ""

	mutate := func() error {
		// Determine the authoritative source for building the Approval spec.
		// Scoped requests load a validated fresh copy from the API server (which
		// also enforces the ambiguity guard) BEFORE the idempotency check so that
		// an already-bound request cannot bypass the guard.
		// Unscoped requests run the idempotency and liveness check instead.
		source := approvalReq
		if isKeyed {
			scopedSource, skip, scopedErr := h.resolveScopedGrantSource(ctx, approvalObj, approvalReq)
			if scopedErr != nil || skip {
				return scopedErr
			}
			source = scopedSource
		} else {
			skip, unscopedErr := h.checkUnscopedGrantSource(ctx, approvalObj, approvalReq)
			if unscopedErr != nil || skip {
				return unscopedErr
			}
		}

		// A scoped Approval this request was already bound to was deleted; an
		// automatic grant must not recreate it, as that would undo a revocation.
		if isKeyed && approvalObj.UID == "" && source.Spec.Strategy == approvalv1.ApprovalStrategyAuto &&
			approvalReq.Status.Approval.UID != "" {
			return errScopedApprovalDeleted
		}

		// A lifted revocation keeps the decisions that led to it.
		decisions := source.Spec.Decisions
		if isKeyed && isScopedApprovalRevoked(approvalObj) {
			decisions = approvalv1.TrimDecisions(append(slices.Clone(approvalObj.Spec.Decisions), source.Spec.Decisions...))
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
			Decisions: decisions,

			ApprovedRequest: types.ObjectRefFromObject(source),
			ApprovalKey:     source.Spec.ApprovalKey,
		}

		// copy labels from the validated approval request.
		approvalObj.Labels = maps.Clone(source.Labels)
		// Spec-derived labels always reflect the request spec, never a copied value.
		approvalv1.SetApprovalLabels(approvalObj, source.Spec.Target,
			source.Spec.Requester.TeamName,
			source.Spec.Decider.TeamName,
			source.Spec.Action,
			string(source.Spec.Strategy))

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, approvalObj, mutate)
	if stderrors.Is(err, errScopedApprovalDeleted) {
		approvalReq.SetCondition(approval_condition.NewApprovedCondition())
		approvalReq.SetCondition(condition.NewDoneProcessingCondition("Request has been approved automatically, but its approval was deleted"))
		approvalReq.SetCondition(condition.NewNotReadyCondition(builder.ReasonApprovalRevoked,
			"Request has been approved automatically, but its approval was deleted after it was issued and is not recreated"))
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "failed to create or update approval")
	}

	approvalReq.Status.Approval = *types.ObjectRefFromObject(approvalObj)

	approvalReq.SetCondition(approval_condition.NewApprovedCondition())
	if isKeyed && isScopedApprovalRevoked(approvalObj) {
		approvalReq.SetCondition(condition.NewDoneProcessingCondition("Request has been approved, but the approval stays revoked"))
		approvalReq.SetCondition(condition.NewNotReadyCondition(builder.ReasonApprovalRevoked,
			"Request has been approved, but the approval stays revoked; the decider must grant the approval again"))
		return nil
	}
	approvalReq.SetCondition(condition.NewDoneProcessingCondition("Request has been approved"))
	approvalReq.SetCondition(
		condition.NewReadyCondition("Granted", "Request has been approved and approval is granted"))

	return nil
}

// resolveScopedGrantSource validates an existing scoped Approval against the
// request and returns the validated grant source. skip is true when the mutation
// must leave the Approval untouched (revocation preserved or already processed).
func (h *ApprovalRequestHandler) resolveScopedGrantSource(ctx context.Context, approvalObj *approvalv1.Approval, approvalReq *approvalv1.ApprovalRequest) (source *approvalv1.ApprovalRequest, skip bool, err error) {
	logger := log.FromContext(ctx)

	// For scoped Approvals that already exist, validate identity and protect revocations.
	if approvalObj.UID != "" {
		if approvalObj.Spec.ApprovalKey != approvalReq.Spec.ApprovalKey {
			return nil, false, fmt.Errorf("scoped approval %s: approvalKey mismatch: existing %q != request %q",
				approvalObj.Name, approvalObj.Spec.ApprovalKey, approvalReq.Spec.ApprovalKey)
		}
		if !approvalv1.ScopedIdentityMatch(approvalObj.Spec.Target, approvalReq.Spec.Target,
			approvalObj.Spec.ApprovalKey, approvalReq.Spec.ApprovalKey) {
			return nil, false, fmt.Errorf("scoped approval %s: target identity mismatch", approvalObj.Name)
		}

		// Preserve active revocations: do not overwrite Rejected/Suspended with a new
		// grant, unless a person granted a newer request.
		if isScopedApprovalRevoked(approvalObj) && !liftsScopedRevocation(approvalObj, approvalReq) {
			logger.Info("Scoped approval is revoked; preserving revocation",
				"state", approvalObj.Spec.State)
			return nil, true, nil
		}

		// An existing scoped Approval must already be controlled by its target; this
		// controller always writes that reference on creation. Refuse to adopt or
		// overwrite one with missing or foreign authority.
		if ref := metav1.GetControllerOfNoCopy(approvalObj); ref == nil || ref.UID != approvalReq.Spec.Target.UID {
			return nil, false, fmt.Errorf("scoped approval %s: missing or foreign controller owner", approvalObj.Name)
		}
	}

	source, err = h.loadSoleLiveScopedGrantSource(ctx, approvalReq)
	if err != nil {
		return nil, false, fmt.Errorf("scoped grant-source selection: %w", err)
	}

	// Scoped idempotency: check using the validated source's identity.
	if approvalObj.Spec.ApprovedRequest != nil &&
		approvalObj.Spec.ApprovedRequest.Name == source.Name &&
		approvalObj.Spec.ApprovedRequest.Namespace == source.Namespace &&
		approvalObj.Spec.ApprovedRequest.UID == source.UID {
		logger.Info("Approval has already been processed for this request")
		return nil, true, nil
	}

	return source, false, nil
}

// isScopedApprovalRevoked reports whether a scoped Approval is revoked: Rejected,
// Suspended, Expired after a suspension, or Semigranted, i.e. with the first of
// two approvals of a re-grant after a revocation.
func isScopedApprovalRevoked(approvalObj *approvalv1.Approval) bool {
	switch approvalObj.Spec.State {
	case approvalv1.ApprovalStateRejected, approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateSemigranted:
		return true
	case approvalv1.ApprovalStateExpired:
		return approvalObj.IsExpiredAfterSuspension()
	default:
		return false
	}
}

// errScopedApprovalDeleted marks an automatic grant whose scoped Approval was
// deleted after this request had been bound to it.
var errScopedApprovalDeleted = stderrors.New("scoped approval was deleted after it was issued")

// liftsScopedRevocation reports whether approvalReq replaces the revocation of a
// scoped Approval: only a request other than the one the Approval was granted for,
// fully granted by a person after the revocation; under FourEyes both approvals
// must follow it. Times come from decision timestamps; when one is unknown the
// revocation is kept. A Semigranted Approval counts as revoked when its first
// re-grant approval was given. A timestamp is set
// by its writer (the control plane API uses server time) or, when empty,
// defaulted by the mutating webhook, so other writers can supply any value. An
// automatic grant keeps the revocation, and so does a FourEyes request that is
// only Semigranted, as it never reaches the grant path.
func liftsScopedRevocation(approvalObj *approvalv1.Approval, approvalReq *approvalv1.ApprovalRequest) bool {
	if approvalReq.Spec.Strategy == approvalv1.ApprovalStrategyAuto || approvalReq.Spec.State != approvalv1.ApprovalStateGranted {
		return false
	}
	if bound := approvalObj.Spec.ApprovedRequest; bound != nil && bound.UID == approvalReq.UID {
		return false
	}
	grantedAt := grantTime(approvalReq.Spec.Strategy, approvalReq.Spec.Decisions)
	revokedAt := revocationTime(approvalObj)
	return grantedAt != nil && revokedAt != nil && grantedAt.After(revokedAt.Time)
}

// grantTime returns the timestamp of the earliest approval of the latest grant:
// the latest decision resulting in Granted and, under FourEyes, the Semigranted
// decision before it. It returns nil when a decision is missing or has no
// timestamp.
func grantTime(strategy approvalv1.ApprovalStrategy, decisions []approvalv1.Decision) *metav1.Time {
	for i := len(decisions) - 1; i >= 0; i-- {
		if decisions[i].ResultingState != approvalv1.ApprovalStateGranted {
			continue
		}
		if strategy != approvalv1.ApprovalStrategyFourEyes {
			return decisions[i].Timestamp
		}
		if i == 0 || decisions[i-1].ResultingState != approvalv1.ApprovalStateSemigranted ||
			decisions[i].Timestamp == nil || decisions[i-1].Timestamp == nil {
			return nil
		}
		if decisions[i].Timestamp.Before(decisions[i-1].Timestamp) {
			return decisions[i].Timestamp
		}
		return decisions[i-1].Timestamp
	}
	return nil
}

// revocationTime returns when a revoked scoped Approval was revoked: the
// timestamp of its last decision, when that decision is the revocation, i.e. it
// results in the current state, or in Suspended followed only by expiry. It
// returns nil when the state was changed without a decision, as an older
// revocation decision would then date a newer revocation.
func revocationTime(approvalObj *approvalv1.Approval) *metav1.Time {
	decisions := approvalObj.Spec.Decisions
	i := len(decisions) - 1
	revokedState := approvalObj.Spec.State
	if revokedState == approvalv1.ApprovalStateExpired {
		for i >= 0 && decisions[i].ResultingState == approvalv1.ApprovalStateExpired {
			i--
		}
		revokedState = approvalv1.ApprovalStateSuspended
	}
	if i < 0 || decisions[i].ResultingState != revokedState {
		return nil
	}
	return decisions[i].Timestamp
}

// checkUnscopedGrantSource applies the unscoped idempotency check and verifies the
// source request is still live and granted. skip is true when the request has
// already been processed.
func (h *ApprovalRequestHandler) checkUnscopedGrantSource(ctx context.Context, approvalObj *approvalv1.Approval, approvalReq *approvalv1.ApprovalRequest) (skip bool, err error) {
	// Unscoped idempotency check.
	if approvalObj.Spec.ApprovedRequest != nil && approvalObj.Spec.ApprovedRequest.Name == approvalReq.Name {
		log.FromContext(ctx).Info("Approval has already been processed for this request")
		return true, nil
	}

	// Unscoped: re-read via uncached reader to verify liveness.
	freshAR := &approvalv1.ApprovalRequest{}
	if err = h.Reader.Get(ctx, ctrlclient.ObjectKeyFromObject(approvalReq), freshAR); err != nil {
		return false, fmt.Errorf("re-reading source request: %w", err)
	}
	if freshAR.UID != approvalReq.UID {
		return false, fmt.Errorf("source request was recreated (expected UID %s, got %s)", approvalReq.UID, freshAR.UID)
	}
	if freshAR.DeletionTimestamp != nil {
		return false, fmt.Errorf("source request is terminating")
	}
	if freshAR.Spec.State != approvalv1.ApprovalStateGranted {
		return false, fmt.Errorf("source request is no longer granted (state: %s)", freshAR.Spec.State)
	}

	return false, nil
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
