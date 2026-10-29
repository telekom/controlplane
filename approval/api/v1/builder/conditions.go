// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

var (
	ConditionTypeApprovalGranted = "ApprovalGranted"
)

// Ready condition reasons set by consumers of the approval builder.
const (
	// ReasonApprovalPending indicates the resource is waiting for an approval decision.
	ReasonApprovalPending = "ApprovalPending"
	// ReasonApprovalDenied indicates the approval has been denied.
	ReasonApprovalDenied = "ApprovalDenied"
	// ReasonApprovalRequestDenied indicates the approval request itself was denied.
	ReasonApprovalRequestDenied = "ApprovalRequestDenied"
)

// ReasonApprovalRevoked is the Ready reason the approval controller sets on a
// granted scoped ApprovalRequest whose Approval it does not issue, because the
// Approval stays revoked or was deleted after it was issued.
const ReasonApprovalRevoked = "Revoked"

// ConditionTypeForKey returns the condition type for the given approval key.
// An empty key returns the canonical unscoped type; the provider and consumer
// keys return ApprovalGrantedProvider and ApprovalGrantedConsumer, so that two
// gates on the same owner use distinct condition types.
func ConditionTypeForKey(key string) string {
	switch key {
	case approvalv1.ApprovalKeyProvider:
		return ConditionTypeApprovalGranted + "Provider"
	case approvalv1.ApprovalKeyConsumer:
		return ConditionTypeApprovalGranted + "Consumer"
	default:
		// Unscoped. The scoped builder rejects any other key before it sets a condition.
		return ConditionTypeApprovalGranted
	}
}

// approvalIssueTimeout bounds how long a granted ApprovalRequest may wait for
// the approval controller to issue its Approval before SetPendingConditions
// blocks the owner.
const approvalIssueTimeout = 5 * time.Minute

// approvalIssueRequeueCap caps the requeue while the owner waits for the
// Approval, so a missed watch event is noticed and the bound is applied on time.
const approvalIssueRequeueCap = 30 * time.Second

// approvalIssueRequeueJitter is the jitter factor of that requeue. It is kept
// below config.JitterFactor so the bound is applied at most 10% late.
const approvalIssueRequeueJitter = 0.1

// maxRequestErrorLength caps the error of the ApprovalRequest that
// SetPendingConditions appends to the owner's message.
const maxRequestErrorLength = 256

const messageApprovalNotIssuedYet = "Approval granted, waiting for the Approval to be issued"

func newApprovalGrantedCondition(state approvalv1.ApprovalState, msg string) metav1.Condition {
	return newKeyedApprovalGrantedCondition("", state, msg)
}

func newKeyedApprovalGrantedCondition(key string, state approvalv1.ApprovalState, msg string) metav1.Condition {
	cond := metav1.Condition{
		Type:               ConditionTypeForKey(key),
		Status:             metav1.ConditionFalse,
		Reason:             string(state),
		Message:            msg,
		LastTransitionTime: metav1.Now(),
	}
	if state == approvalv1.ApprovalStateGranted {
		cond.Status = metav1.ConditionTrue
	}

	return cond
}

// SetPendingConditions sets the Ready and Processing conditions of obj for an
// ApprovalResultPending result of ab. message describes the wait for a decision.
//
// A request that still waits for a decider blocks obj with ReasonApprovalPending.
// A granted request (always the case for Auto) only waits for the approval
// controller to issue its Approval, so obj is processing and requeued. Once
// approvalIssueTimeout has passed since the grant, obj is blocked with
// ReasonApprovalPending and a message that points at the approval controller.
func SetPendingConditions(ctx context.Context, obj types.Object, ab ApprovalBuilder, message string) {
	request := ab.GetApprovalRequest()
	if request == nil || request.Spec.State != approvalv1.ApprovalStateGranted {
		obj.SetCondition(condition.NewNotReadyCondition(ReasonApprovalPending, message))
		obj.SetCondition(condition.NewBlockedCondition(message))
		return
	}

	granted := grantedAt(request)
	if remaining := approvalIssueTimeout - time.Since(granted); remaining > 0 {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonProcessing, messageApprovalNotIssuedYet))
		obj.SetCondition(condition.NewProcessingCondition(condition.ReasonProcessing, messageApprovalNotIssuedYet))
		requeue := min(remaining, approvalIssueRequeueCap)
		// #nosec G404 -- This is not a cryptographic use case
		contextutil.SetRequeueAfter(ctx, time.Duration(float64(requeue)*(1+approvalIssueRequeueJitter*rand.Float64())))
		return
	}

	// The message holds the grant time, not the elapsed time, so the conditions
	// stay the same on every reconcile past the bound.
	notIssued := fmt.Sprintf("ApprovalRequest %s was granted at %s, but no Approval was issued; check the approval controller",
		request.Name, granted.UTC().Format(time.RFC3339))
	if ready := meta.FindStatusCondition(request.Status.Conditions, condition.ConditionTypeReady); ready != nil &&
		ready.Status == metav1.ConditionFalse && ready.Reason == condition.ReasonError &&
		ready.Message != "" && ready.ObservedGeneration >= request.Generation {
		notIssued += ": " + truncate(ready.Message, maxRequestErrorLength)
	}
	log.FromContext(ctx).Info("Approval not issued within timeout", "approvalRequest", request.Name, "grantedAt", granted)
	obj.SetCondition(condition.NewNotReadyCondition(ReasonApprovalPending, notIssued))
	obj.SetCondition(condition.NewBlockedCondition(notIssued))
}

// truncate cuts s to at most n runes and marks a cut with an ellipsis.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// grantedAt returns the time of the latest decision that granted request. It
// falls back to the creation time of request, and to now if neither is known.
//
// A Granted decision without a timestamp is skipped, so a request granted only
// by such a decision falls back to its creation time and the bound may already
// be reached.
func grantedAt(request *approvalv1.ApprovalRequest) time.Time {
	var latest time.Time
	for _, d := range request.Spec.Decisions {
		if d.ResultingState == approvalv1.ApprovalStateGranted && d.Timestamp != nil && d.Timestamp.After(latest) {
			latest = d.Timestamp.Time
		}
	}
	if latest.IsZero() {
		latest = request.CreationTimestamp.Time
	}
	if latest.IsZero() {
		latest = time.Now()
	}
	return latest
}

// ClearApprovalPendingReady updates the Ready condition on the object if it is
// still set by SetPendingConditions. This should be called after approval is
// granted to ensure downstream blockers don't leave a stale Ready reason.
// Returns true if the condition was updated.
func ClearApprovalPendingReady(obj types.Object) bool {
	ready := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	if ready != nil && (ready.Reason == ReasonApprovalPending || ready.Message == messageApprovalNotIssuedYet) {
		return obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonProcessing, "Approval granted, provisioning in progress"))
	}
	return false
}
