// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

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

// approvalResult carries the outcome of the approval check.
type approvalResult struct {
	result           builder.ApprovalResult
	providerApproval *ctypes.ObjectRef
}

// ensureApprovals creates or reconciles the provider ApprovalRequest that gates
// this Listener: requester=listenerTeam, decider=providerTeam. The provider owns
// the API whose traffic is captured, so its consent is the security control.
//
// Strategy is Auto if requester and decider are on the same team, Simple otherwise.
//
// The authorization fingerprint (derived from the canonical capture intent) is
// passed to WithHashValue so that the ApprovalRequest name changes when the
// intent changes, forcing re-approval.
//
// NOTE: only ONE approval may be built per owner CR. The approval builder ends
// Build() with a janitor Cleanup scoped to the owner UID, which deletes every
// owner-owned ApprovalRequest it is not itself tracking. A second builder on the
// same Listener therefore deletes the first one's request on every reconcile,
// and both would collapse onto the same Approval CR because ApprovalName()
// derives from owner kind + name only. A consumer-side decision must not be
// modelled as a second builder here.
func (h *ListenerHandler) ensureApprovals(
	ctx context.Context,
	listener *spectrev1.Listener,
	consumerApp *applicationv1.Application,
	providerApp *applicationv1.Application,
	intent *authorizationIntent,
) (*approvalResult, error) {
	logger := log.FromContext(ctx)

	result := &approvalResult{}

	providerRes, err := h.buildApproval(ctx, listener, consumerApp, providerApp, intent)
	if err != nil {
		return nil, err
	}
	result.providerApproval = ctypes.ObjectRefFromObject(providerRes.builder.GetApproval())
	result.result = providerRes.result

	// --- Evaluate gate ---
	switch providerRes.result {
	case builder.ApprovalResultGranted:
		builder.ClearApprovalPendingReady(listener)
		return result, nil

	case builder.ApprovalResultDenied:
		logger.Info("Approval denied", "provider", providerRes.result)
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "Approval has been denied"))
		listener.SetCondition(condition.NewDoneProcessingCondition("Approval has been denied"))
		return result, nil

	case builder.ApprovalResultRequestDenied:
		logger.Info("ApprovalRequest denied", "provider", providerRes.result)
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "ApprovalRequest has been denied"))
		listener.SetCondition(condition.NewDoneProcessingCondition("ApprovalRequest has been denied"))
		return result, nil

	case builder.ApprovalResultPending:
		logger.Info("Approval pending", "provider", providerRes.result)
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonApprovalPending, "Waiting for approval decision"))
		listener.SetCondition(condition.NewBlockedCondition("Waiting for approval decision"))
		return result, nil

	default:
		// Unknown result: fail closed rather than silently waiting forever.
		return nil, errors.Errorf("unknown approval-builder result %q", providerRes.result)
	}
}

type singleApprovalResult struct {
	result  builder.ApprovalResult
	builder builder.ApprovalBuilder
}

func (h *ListenerHandler) buildApproval(
	ctx context.Context,
	listener *spectrev1.Listener,
	consumerApp *applicationv1.Application,
	providerApp *applicationv1.Application,
	intent *authorizationIntent,
) (*singleApprovalResult, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	strategy := computeStrategy(consumerApp.Spec.Team, providerApp.Spec.Team)

	consumerRef := ctypes.TypedObjectRefFromObject(consumerApp, c.Scheme())
	providerRef := ctypes.TypedObjectRefFromObject(providerApp, c.Scheme())

	requester := &approvalapi.Requester{
		TeamName:       consumerApp.Spec.Team,
		TeamEmail:      consumerApp.Spec.TeamEmail,
		ApplicationRef: consumerRef,
	}
	if err := requester.SetProperties(intent.approvalProperties()); err != nil {
		return nil, errors.Wrap(err, "failed to set requester properties")
	}

	decider := &approvalapi.Decider{
		TeamName:       providerApp.Spec.Team,
		TeamEmail:      providerApp.Spec.TeamEmail,
		ApplicationRef: providerRef,
	}

	fingerprint := intent.fingerprint()

	ab := builder.NewApprovalBuilder(c, listener)
	ab.WithAction("listen-provider")
	ab.WithHashValue(fingerprint)
	ab.WithRequester(requester)
	ab.WithDecider(decider)
	ab.WithStrategy(strategy)

	res, err := ab.Build(ctx)
	if err != nil {
		return nil, err
	}

	return &singleApprovalResult{result: res, builder: ab}, nil
}

// computeStrategy returns Auto if both teams are the same, Simple otherwise.
func computeStrategy(requesterTeam, deciderTeam string) approvalapi.ApprovalStrategy {
	if requesterTeam == deciderTeam {
		return approvalapi.ApprovalStrategyAuto
	}
	return approvalapi.ApprovalStrategySimple
}
