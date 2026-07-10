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
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(obj.Spec.BasePath),
	}); err != nil {
		return errors.Wrapf(err, "failed to list McpServers for basePath %q", obj.Spec.BasePath)
	}

	// Filter to exact basePath match
	var candidates []agenticv1.McpServer
	for i := range serverList.Items {
		if serverList.Items[i].Spec.BasePath == obj.Spec.BasePath {
			candidates = append(candidates, serverList.Items[i])
		}
	}

	// Determine active: oldest-wins semantics.
	// Use SortStableFunc with namespace as tiebreaker for equal timestamps,
	// ensuring deterministic ordering even in the (unlikely) same-millisecond case.
	slices.SortStableFunc(candidates, func(a, b agenticv1.McpServer) int {
		c := a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		if c == 0 {
			return cmp.Compare(a.GetNamespace(), b.GetNamespace())
		}
		return c
	})

	if ctypes.Equals(&candidates[0], obj) {
		// This server is the active one
		obj.Status.Active = true
		obj.SetCondition(condition.NewReadyCondition("McpServerActive", "McpServer is active"))
		obj.SetCondition(condition.NewDoneProcessingCondition("McpServer is processed"))
		logger.Info("McpServer is processed")
	} else {
		// Another server already owns this basePath
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("McpServerNotActive", "McpServer is not active"))
		obj.SetCondition(condition.NewBlockedCondition(
			fmt.Sprintf("McpServer is blocked, another McpServer with the same BasePath %q is active. "+
				"It will be automatically processed, if the other McpServer will be deleted.", obj.Spec.BasePath),
		))
		logger.Info("McpServer is blocked, another McpServer with the same BasePath is already active.")
	}

	return nil
}

func (h *McpServerHandler) Delete(ctx context.Context, obj *agenticv1.McpServer) error {
	// No owned resources to clean up.
	// Other McpServers for the same basePath will be re-reconciled via
	// MapMcpServerToMcpServer watch, allowing standby to become active.
	return nil
}
