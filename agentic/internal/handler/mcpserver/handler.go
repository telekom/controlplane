// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

var _ handler.Handler[*agenticv1.McpServer] = &McpServerHandler{}

type McpServerHandler struct{}

func (h *McpServerHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.McpServer) error {
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

	// Determine active: oldest-wins semantics
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreationTimestamp.Before(&candidates[j].CreationTimestamp)
	})

	if len(candidates) > 0 && candidates[0].UID != obj.UID {
		// Another server already owns this basePath
		obj.Status.Active = false
		msg := fmt.Sprintf("BasePath %q is already registered by another McpServer.", obj.Spec.BasePath)
		obj.SetCondition(condition.NewNotReadyCondition("McpServerAlreadyExists", msg))
		obj.SetCondition(condition.NewBlockedCondition(msg + " McpServer will be automatically processed when the existing one is deleted"))
		return nil
	}

	// This server is the active one
	obj.Status.Active = true

	obj.SetCondition(condition.NewReadyCondition("McpServerRegistered",
		"McpServer has been registered"))
	obj.SetCondition(condition.NewDoneProcessingCondition(
		"McpServer has been registered"))

	return nil
}

func (h *McpServerHandler) Delete(ctx context.Context, obj *agenticv1.McpServer) error {
	// No owned resources to clean up.
	// Other McpServers for the same basePath will be re-reconciled via
	// MapMcpServerToMcpServer watch, allowing standby to become active.
	return nil
}
