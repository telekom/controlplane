// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// approvalOutcome is a fail-closed aggregate status for dual-gate approval.
// The zero value (outcomeUnknown) never permits provisioning.
type approvalOutcome int

const (
	outcomeUnknown       approvalOutcome = iota // zero value: never permits provisioning
	outcomeGranted                              // both gates grant the exact current intent
	outcomePending                              // no errors/denials, but at least one gate not yet granted
	outcomeRequestDenied                        // a required ApprovalRequest was rejected
	outcomeDenied                               // an Approval was rejected/suspended/denied by expiry
	outcomeError                                // gate read/identity/hash/build failure or unknown outcome
)

// String returns a human-readable label for the outcome.
func (o approvalOutcome) String() string {
	switch o {
	case outcomeUnknown:
		return "Unknown"
	case outcomeGranted:
		return "Granted"
	case outcomePending:
		return "Pending"
	case outcomeRequestDenied:
		return "RequestDenied"
	case outcomeDenied:
		return "Denied"
	case outcomeError:
		return "Error"
	default:
		return fmt.Sprintf("approvalOutcome(%d)", int(o))
	}
}

// gateResult carries the outcome of a single approval gate.
type gateResult struct {
	outcome         approvalOutcome
	approval        *ctypes.ObjectRef
	approvalRequest *ctypes.ObjectRef
	err             error
}

// dualApprovalResult carries the aggregate outcome of both gates.
type dualApprovalResult struct {
	outcome  approvalOutcome
	provider *gateResult
	consumer *gateResult
	err      error // combined diagnostics
}

// ensureApprovals creates or reconciles both the provider and consumer
// ApprovalRequests that gate this Listener.
//
// Provider gate: requester=observer(A), decider=provider(P), action="listen-provider"
// Consumer gate: requester=observer(A), decider=consumer(C), action="listen-consumer"
//
// Both builders use WithApprovalKey(key), enabling multiple builders per owner
// CR via Phase 1's keyed scoped cleanup.
//
// On partial failure, the populated result AND the error are returned so
// callers can process known denials before returning the combined error.
func (h *ListenerHandler) ensureApprovals(
	ctx context.Context,
	listener *spectrev1.Listener,
	observerApp *applicationv1.Application,
	consumerApp *applicationv1.Application,
	providerApp *applicationv1.Application,
	intent *authorizationIntent,
) (*dualApprovalResult, error) {
	// Build both gates independently — do NOT stop after the first error.
	providerGate := h.buildScopedGate(ctx, listener, observerApp, providerApp, intent,
		"provider", "listen-provider")
	consumerGate := h.buildScopedGate(ctx, listener, observerApp, consumerApp, intent,
		"consumer", "listen-consumer")

	// Write all four status references before evaluating outcome.
	listener.Status.ProviderApproval = providerGate.approval
	listener.Status.ConsumerApproval = consumerGate.approval
	listener.Status.ProviderApprovalRequest = providerGate.approvalRequest
	listener.Status.ConsumerApprovalRequest = consumerGate.approvalRequest

	// Compute aggregate outcome.
	result := &dualApprovalResult{
		provider: providerGate,
		consumer: consumerGate,
	}
	result.outcome = computeAggregateOutcome(providerGate.outcome, consumerGate.outcome)

	// Combine errors.
	var combinedErr error
	switch {
	case providerGate.err != nil && consumerGate.err != nil:
		combinedErr = fmt.Errorf("provider gate: %w; consumer gate: %v", providerGate.err, consumerGate.err)
	case providerGate.err != nil:
		combinedErr = fmt.Errorf("provider gate: %w", providerGate.err)
	case consumerGate.err != nil:
		combinedErr = fmt.Errorf("consumer gate: %w", consumerGate.err)
	}
	result.err = combinedErr

	// Set conditions based on aggregate outcome.
	h.setAggregateConditions(ctx, listener, result)

	if combinedErr != nil {
		return result, combinedErr
	}
	return result, nil
}

// buildScopedGate builds a single approval gate using the keyed builder.
// It validates the returned Approval's controller owner is this Listener
// (identity hardening per section 2.6).
func (h *ListenerHandler) buildScopedGate(
	ctx context.Context,
	listener *spectrev1.Listener,
	requesterApp *applicationv1.Application,
	deciderApp *applicationv1.Application,
	intent *authorizationIntent,
	key string,
	action string,
) *gateResult {
	c := cclient.ClientFromContextOrDie(ctx)
	result := &gateResult{}

	strategy := computeStrategy(requesterApp.Spec.Team, deciderApp.Spec.Team)

	requesterRef := ctypes.TypedObjectRefFromObject(requesterApp, c.Scheme())
	deciderRef := ctypes.TypedObjectRefFromObject(deciderApp, c.Scheme())

	requester := &approvalapi.Requester{
		TeamName:       requesterApp.Spec.Team,
		TeamEmail:      requesterApp.Spec.TeamEmail,
		ApplicationRef: requesterRef,
	}
	if err := requester.SetProperties(intent.gateApprovalProperties(action)); err != nil {
		result.outcome = outcomeError
		result.err = errors.Wrapf(err, "failed to set requester properties for %s gate", key)
		return result
	}

	decider := &approvalapi.Decider{
		TeamName:       deciderApp.Spec.Team,
		TeamEmail:      deciderApp.Spec.TeamEmail,
		ApplicationRef: deciderRef,
	}

	hashValue, err := intent.gateRequestHash(key, requesterApp.Spec.Team, deciderApp.Spec.Team)
	if err != nil {
		result.outcome = outcomeError
		result.err = errors.Wrapf(err, "failed to compute gate request hash for %s gate", key)
		return result
	}

	ab := builder.NewApprovalBuilder(c, listener)
	ab.WithApprovalKey(key)
	ab.WithAction(action)
	ab.WithHashValue(hashValue)
	ab.WithRequester(requester)
	ab.WithDecider(decider)
	ab.WithStrategy(strategy)

	buildResult, err := ab.Build(ctx)
	if err != nil {
		result.outcome = outcomeError
		result.err = errors.Wrapf(err, "failed to build %s gate", key)
		// Still populate refs if available.
		if ab.GetApproval() != nil && ab.GetApproval().Name != "" {
			result.approval = ctypes.ObjectRefFromObject(ab.GetApproval())
		}
		if ab.GetApprovalRequest() != nil && ab.GetApprovalRequest().Name != "" {
			result.approvalRequest = ctypes.ObjectRefFromObject(ab.GetApprovalRequest())
		}
		return result
	}

	// Populate refs.
	if ab.GetApproval() != nil && ab.GetApproval().Name != "" {
		result.approval = ctypes.ObjectRefFromObject(ab.GetApproval())
	}
	if ab.GetApprovalRequest() != nil && ab.GetApprovalRequest().Name != "" {
		result.approvalRequest = ctypes.ObjectRefFromObject(ab.GetApprovalRequest())
	}

	// Identity hardening (section 2.6): verify the Approval's controller owner
	// is this Listener. The builder validates scoped identity (ScopedIdentityMatch,
	// isScopedGrantBound), but the handler also verifies ownership.
	if approval := ab.GetApproval(); approval != nil && approval.Name != "" {
		ownerRefs := approval.GetOwnerReferences()
		ownerFound := false
		for _, ref := range ownerRefs {
			if ref.UID == listener.UID && ref.Controller != nil && *ref.Controller {
				ownerFound = true
				break
			}
		}
		if len(ownerRefs) == 0 {
			// No owner references yet — the Approval controller has not
			// reconciled (adopted) this Approval. Log a warning but accept;
			// the builder's ScopedIdentityMatch already verified the target.
			logger := log.FromContext(ctx)
			logger.V(1).Info("Approval has no owner references; not yet adopted by controller",
				"gate", key, "approval", approval.Name)
		} else if !ownerFound {
			result.outcome = outcomeError
			result.err = fmt.Errorf("%s gate: Approval %s/%s is not owned by Listener %s/%s",
				key, approval.Namespace, approval.Name, listener.Namespace, listener.Name)
			return result
		}
	}

	// Map builder result to outcome.
	switch buildResult {
	case builder.ApprovalResultGranted:
		result.outcome = outcomeGranted
	case builder.ApprovalResultPending:
		result.outcome = outcomePending
	case builder.ApprovalResultDenied:
		result.outcome = outcomeDenied
	case builder.ApprovalResultRequestDenied:
		result.outcome = outcomeRequestDenied
	default:
		result.outcome = outcomeError
		result.err = fmt.Errorf("unknown approval-builder result %q for %s gate", buildResult, key)
	}

	return result
}

// computeAggregateOutcome applies the dual-gate aggregate table (fail-closed).
//
// Priority (highest to lowest):
//  1. Either gate Denied -> outcomeDenied
//  2. Either gate RequestDenied -> outcomeRequestDenied
//  3. Any gate Error or Unknown -> outcomeError
//  4. Either gate Pending -> outcomePending
//  5. Both Granted -> outcomeGranted
func computeAggregateOutcome(provider, consumer approvalOutcome) approvalOutcome {
	// 1. Denial takes precedence over everything (enables cleanup).
	if provider == outcomeDenied || consumer == outcomeDenied {
		return outcomeDenied
	}

	// 2. Request denial.
	if provider == outcomeRequestDenied || consumer == outcomeRequestDenied {
		return outcomeRequestDenied
	}

	// 3. Error or unknown (fail-closed).
	if provider == outcomeError || consumer == outcomeError ||
		provider == outcomeUnknown || consumer == outcomeUnknown {
		return outcomeError
	}

	// 4. Pending.
	if provider == outcomePending || consumer == outcomePending {
		return outcomePending
	}

	// 5. Both granted.
	if provider == outcomeGranted && consumer == outcomeGranted {
		return outcomeGranted
	}

	// Unreachable if all outcomes are covered, but fail-closed.
	return outcomeError
}

// setAggregateConditions updates the Listener conditions based on the aggregate
// dual-gate result. ClearApprovalPendingReady is called only after BOTH gates
// allow provisioning.
func (h *ListenerHandler) setAggregateConditions(ctx context.Context, listener *spectrev1.Listener, result *dualApprovalResult) {
	logger := log.FromContext(ctx)

	switch result.outcome {
	case outcomeGranted:
		builder.ClearApprovalPendingReady(listener)

	case outcomeDenied:
		gate := "provider"
		if result.consumer != nil && result.consumer.outcome == outcomeDenied {
			gate = "consumer"
		}
		logger.Info("Approval denied", "gate", gate)
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied,
			fmt.Sprintf("Approval has been denied (%s gate)", gate)))
		listener.SetCondition(condition.NewDoneProcessingCondition(
			fmt.Sprintf("Approval has been denied (%s gate)", gate)))

	case outcomeRequestDenied:
		gate := "provider"
		if result.consumer != nil && result.consumer.outcome == outcomeRequestDenied {
			gate = "consumer"
		}
		logger.Info("ApprovalRequest denied", "gate", gate)
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied,
			fmt.Sprintf("ApprovalRequest has been denied (%s gate)", gate)))
		listener.SetCondition(condition.NewDoneProcessingCondition(
			fmt.Sprintf("ApprovalRequest has been denied (%s gate)", gate)))

	case outcomePending:
		logger.Info("Approval pending")
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonApprovalPending,
			"Waiting for approval decision"))
		listener.SetCondition(condition.NewBlockedCondition("Waiting for approval decision"))

	case outcomeError, outcomeUnknown:
		logger.Info("Approval evaluation error or unknown", "outcome", result.outcome.String())
		// Conditions are not set for errors — the caller returns the error
		// and the controller framework handles requeue.
	}
}

// computeStrategy returns Auto if both teams are the same and neither is empty,
// Simple otherwise. Empty or unresolved team identity must never produce Auto.
func computeStrategy(requesterTeam, deciderTeam string) approvalapi.ApprovalStrategy {
	if requesterTeam == "" || deciderTeam == "" {
		return approvalapi.ApprovalStrategySimple
	}
	if requesterTeam == deciderTeam {
		return approvalapi.ApprovalStrategyAuto
	}
	return approvalapi.ApprovalStrategySimple
}
