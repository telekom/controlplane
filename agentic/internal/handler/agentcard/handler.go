// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentcard

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/handler"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

var _ handler.Handler[*agenticv1.AgentCard] = &AgentCardHandler{}

type AgentCardHandler struct{}

func (h *AgentCardHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.AgentCard) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	// List all AgentCards with the same basePath
	cardList := &agenticv1.AgentCardList{}
	if err := c.List(ctx, cardList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: obj.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(obj.Spec.BasePath),
	}); err != nil {
		return errors.Wrapf(err, "failed to list AgentCards for basePath %q", obj.Spec.BasePath)
	}

	if len(cardList.Items) == 0 {
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("AgentCardNotFound",
			"AgentCard was not found among candidates for its basePath; check labels"))
		obj.SetCondition(condition.NewBlockedCondition(
			fmt.Sprintf("AgentCard could not be matched to basePath %q candidates", obj.Spec.BasePath),
		))
		logger.Info("AgentCard not found among candidates, marking not ready")
		return nil
	}

	// Sort all candidates by creation timestamp (oldest-wins).
	// Namespace, name, and kind tiebreakers ensure deterministic ordering for equal timestamps.
	slices.SortStableFunc(cardList.Items, func(a, b agenticv1.AgentCard) int {
		return util.CompareServerRegistrations(&a, &b)
	})

	activeCard := &cardList.Items[0]

	peer, err := util.FindOldestPeerRegistration(ctx, obj)
	if err != nil {
		return errors.Wrap(err, "failed to find competing McpServers")
	}
	var winner client.Object = activeCard
	if ctypes.Equals(activeCard, obj) {
		winner = obj
	}
	if peer != nil && util.CompareServerRegistrations(peer, winner) < 0 {
		winner = peer
	}

	if winner != obj {
		obj.Status.Active = false
		winnerPath, err := util.ServerRegistrationBasePath(winner)
		if err != nil {
			return errors.Wrap(err, "failed to get winning registration basePath")
		}
		if winnerPath != obj.Spec.BasePath {
			conflict := "normalized basePath conflict"
			if strings.EqualFold(winnerPath, obj.Spec.BasePath) {
				conflict = "basePath case conflict"
			}
			msg := fmt.Sprintf("AgentCard is blocked due to a %s: %s with basePath %q has the same normalized basePath as %q. Please change one of the basePaths.",
				conflict, util.ServerRegistrationKind(winner), winnerPath, obj.Spec.BasePath)
			obj.SetCondition(condition.NewNotReadyCondition("AgentCardNotActive", msg))
			obj.SetCondition(condition.NewBlockedCondition(msg))
			logger.Info("AgentCard is blocked by a conflicting registration",
				"conflictingKind", util.ServerRegistrationKind(winner), "basePath", winnerPath)
			return nil
		}
		msg := condition.NewWaitingForTakeoverMessage(
			util.ServerRegistrationKind(winner),
			condition.ConflictIdentifierBasePath,
			obj.Spec.BasePath,
			organizationv1.TeamNameFromNamespace(winner.GetNamespace()),
			condition.NoApplication,
		)
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, msg))
		obj.SetCondition(condition.NewBlockedCondition(msg))
		logger.Info("AgentCard is blocked by an older registration",
			"conflictingKind", util.ServerRegistrationKind(winner), "basePath", winnerPath)
		return nil
	}

	obj.Status.Active = true
	obj.SetCondition(condition.NewReadyCondition("AgentCardActive", "AgentCard is active"))
	obj.SetCondition(condition.NewDoneProcessingCondition("AgentCard is processed"))
	logger.Info("AgentCard is processed")
	return nil
}

func (h *AgentCardHandler) Delete(ctx context.Context, obj *agenticv1.AgentCard) error {
	// No owned resources to clean up.
	// McpServers and AgentCards for the same basePath are re-reconciled by their controller watches.
	return nil
}
