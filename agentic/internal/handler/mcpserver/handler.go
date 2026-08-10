// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

var _ handler.Handler[*agenticv1.McpServer] = &McpServerHandler{}

type McpServerHandler struct{}

func (h *McpServerHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.McpServer) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	// List all McpServers with the same basePath
	serverList := &agenticv1.McpServerList{}
	if err := c.List(ctx, serverList, client.MatchingLabels{
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(obj.Spec.BasePath),
	}); err != nil {
		return errors.Wrapf(err, "failed to list McpServers for basePath %q", obj.Spec.BasePath)
	}

	if len(serverList.Items) == 0 {
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("McpServerNotFound",
			"McpServer was not found among candidates for its basePath; check labels"))
		obj.SetCondition(condition.NewBlockedCondition(
			fmt.Sprintf("McpServer could not be matched to basePath %q candidates", obj.Spec.BasePath)))
		logger.Info("McpServer not found among candidates, marking not ready")
		return nil
	}

	// Sort all candidates by creation timestamp (oldest-wins).
	// Namespace as tiebreaker for equal timestamps ensures deterministic ordering.
	slices.SortStableFunc(serverList.Items, func(a, b agenticv1.McpServer) int {
		c := a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		if c == 0 {
			return cmp.Compare(a.GetNamespace(), b.GetNamespace())
		}
		return c
	})

	activeServer := &serverList.Items[0]

	if ctypes.Equals(activeServer, obj) {
		obj.Status.Active = true
		obj.SetCondition(condition.NewReadyCondition("McpServerActive", "McpServer is active"))
		obj.SetCondition(condition.NewDoneProcessingCondition("McpServer is processed"))
		logger.Info("McpServer is processed")
	} else {
		obj.Status.Active = false

		if obj.Spec.BasePath == activeServer.Spec.BasePath {
			// Exact same basePath — another McpServer is older
			obj.SetCondition(condition.NewNotReadyCondition("McpServerNotActive", "McpServer is not active"))
			obj.SetCondition(condition.NewBlockedCondition(
				fmt.Sprintf("McpServer is blocked, another McpServer with the same BasePath %q is active. "+
					"It will be automatically processed, if the other McpServer will be deleted.", obj.Spec.BasePath),
			))
			logger.Info("McpServer is blocked, another McpServer with the same BasePath is already active.")
		} else {
			// Case conflict (e.g. /MyMcp vs /mymcp)
			obj.SetCondition(condition.NewNotReadyCondition("McpServerNotActive", "McpServer is not active due to case conflict"))
			obj.SetCondition(condition.NewBlockedCondition(
				"McpServer is blocked, another McpServer with the same BasePath but different case is active. " +
					"Please resolve the conflict by changing the BasePath of one of the McpServers.",
			))
			logger.Info("McpServer is blocked, another McpServer with the same BasePath but different case is already active.")
		}
	}

	return nil
}

func (h *McpServerHandler) Delete(ctx context.Context, obj *agenticv1.McpServer) error {
	// No owned resources to clean up.
	// Other McpServers for the same basePath will be re-reconciled via
	// MapMcpServerToMcpServer watch, allowing standby to become active.
	return nil
}
