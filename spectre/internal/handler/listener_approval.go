// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// approvalResult carries the outcome of both approval checks.
type approvalResult struct {
	granted          bool
	providerApproval *ctypes.ObjectRef
	consumerApproval *ctypes.ObjectRef
}

// ensureApprovals creates or reconciles two ApprovalRequests:
//   - Provider approval: requester=listenerTeam, decider=providerTeam
//   - Consumer approval: requester=listenerTeam, decider=consumerTeam
//
// Strategy is Auto if requester and decider are on the same team, Simple otherwise.
// Returns granted=true only when BOTH approvals are Granted.
func (h *ListenerHandler) ensureApprovals(
	ctx context.Context,
	listener *spectrev1.Listener,
	listenerTeam, listenerEmail string,
	providerTeam, providerEmail string,
	consumerTeam, consumerEmail string,
) (*approvalResult, error) {
	logger := log.FromContext(ctx)

	result := &approvalResult{}

	// --- Provider approval ---
	providerRes, err := h.buildApproval(ctx, listener,
		listenerTeam, listenerEmail,
		providerTeam, providerEmail,
		"listen-provider")
	if err != nil {
		return nil, err
	}
	result.providerApproval = ctypes.ObjectRefFromObject(providerRes.builder.GetApproval())

	// --- Consumer approval ---
	consumerRes, err := h.buildApproval(ctx, listener,
		listenerTeam, listenerEmail,
		consumerTeam, consumerEmail,
		"listen-consumer")
	if err != nil {
		return nil, err
	}
	result.consumerApproval = ctypes.ObjectRefFromObject(consumerRes.builder.GetApproval())

	// --- Evaluate gate ---
	if providerRes.result == builder.ApprovalResultDenied || consumerRes.result == builder.ApprovalResultDenied {
		logger.Info("Approval denied", "provider", providerRes.result, "consumer", consumerRes.result)
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "Approval has been denied"))
		listener.SetCondition(condition.NewDoneProcessingCondition("Approval has been denied"))
		return result, nil
	}

	if providerRes.result == builder.ApprovalResultRequestDenied || consumerRes.result == builder.ApprovalResultRequestDenied {
		logger.Info("ApprovalRequest denied", "provider", providerRes.result, "consumer", consumerRes.result)
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "ApprovalRequest has been denied"))
		listener.SetCondition(condition.NewDoneProcessingCondition("ApprovalRequest has been denied"))
		return result, nil
	}

	if providerRes.result != builder.ApprovalResultGranted || consumerRes.result != builder.ApprovalResultGranted {
		logger.Info("Approval pending", "provider", providerRes.result, "consumer", consumerRes.result)
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonApprovalPending, "Waiting for approval decision"))
		listener.SetCondition(condition.NewBlockedCondition("Waiting for approval decision"))
		return result, nil
	}

	// Both granted
	builder.ClearApprovalPendingReady(listener)
	result.granted = true
	return result, nil
}

type singleApprovalResult struct {
	result  builder.ApprovalResult
	builder builder.ApprovalBuilder
}

func (h *ListenerHandler) buildApproval(
	ctx context.Context,
	listener *spectrev1.Listener,
	requesterTeam, requesterEmail string,
	deciderTeam, deciderEmail string,
	action string,
) (*singleApprovalResult, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	strategy := computeStrategy(requesterTeam, deciderTeam)

	requester := &approvalapi.Requester{
		TeamName:  requesterTeam,
		TeamEmail: requesterEmail,
	}
	decider := &approvalapi.Decider{
		TeamName:  deciderTeam,
		TeamEmail: deciderEmail,
	}

	ab := builder.NewApprovalBuilder(c, listener)
	ab.WithAction(action)
	ab.WithHashValue(action)
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
