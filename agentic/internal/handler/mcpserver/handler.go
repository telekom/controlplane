// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver

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

var _ handler.Handler[*agenticv1.McpServer] = &McpServerHandler{}

type McpServerHandler struct{}

func (h *McpServerHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.McpServer) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	// List all McpServers with the same basePath
	serverList := &agenticv1.McpServerList{}
	if err := c.List(ctx, serverList, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: obj.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.BasePathLabelKey:  labelutil.NormalizeLabelValue(obj.Spec.BasePath),
	}); err != nil {
		return errors.Wrapf(err, "failed to list McpServers for basePath %q", obj.Spec.BasePath)
	}

	if len(serverList.Items) == 0 {
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("McpServerNotFound",
			"McpServer was not found among candidates for its basePath; check labels"))
		obj.SetCondition(condition.NewBlockedCondition(
			fmt.Sprintf("McpServer could not be matched to basePath %q candidates", obj.Spec.BasePath),
		))
		logger.Info("McpServer not found among candidates, marking not ready")
		return nil
	}

	// Sort all candidates by creation timestamp (oldest-wins).
	// Namespace, name, and kind tiebreakers ensure deterministic ordering for equal timestamps.
	slices.SortStableFunc(serverList.Items, func(a, b agenticv1.McpServer) int {
		return util.CompareServerRegistrations(&a, &b)
	})

	activeServer := &serverList.Items[0]

	peer, err := util.FindOldestPeerRegistration(ctx, obj)
	if err != nil {
		return errors.Wrap(err, "failed to find competing AgentCards")
	}
	var winner client.Object = activeServer
	if ctypes.Equals(activeServer, obj) {
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
			msg := fmt.Sprintf("McpServer is blocked due to a %s: %s with basePath %q has the same normalized basePath as %q. Please change one of the basePaths.",
				conflict, util.ServerRegistrationKind(winner), winnerPath, obj.Spec.BasePath)
			obj.SetCondition(condition.NewNotReadyCondition("McpServerNotActive", msg))
			obj.SetCondition(condition.NewBlockedCondition(msg))
			logger.Info("McpServer is blocked by a conflicting registration",
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
		logger.Info("McpServer is blocked by an older registration",
			"conflictingKind", util.ServerRegistrationKind(winner), "basePath", winnerPath)
		return nil
	}

	obj.Status.Active = true
	obj.SetCondition(condition.NewReadyCondition("McpServerActive", "McpServer is active"))
	obj.SetCondition(condition.NewDoneProcessingCondition("McpServer is processed"))
	logger.Info("McpServer is processed")
	return nil
}

func (h *McpServerHandler) Delete(ctx context.Context, obj *agenticv1.McpServer) error {
	// No owned resources to clean up.
	// McpServers and AgentCards for the same basePath are re-reconciled by their controller watches.
	return nil
}
