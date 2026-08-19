// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0

import (
	"context"

	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// AgentSpecHandler is a specialized handler for AgentSpecification resources
type AgentSpecHandler struct {
	*common.BaseHandler
}

func NewAgentSpecHandlerInstance() *AgentSpecHandler {
	handler := &AgentSpecHandler{
		BaseHandler: common.NewBaseHandler("tcp.ei.telekom.de/v1", "AgentSpecification", "agentspecifications", 10).WithValidation(common.ValidateObjectName),
	}

	handler.AddHook(common.PreRequestHook, PatchAgentSpecificationRequest)
	return handler
}

func PatchAgentSpecificationRequest(ctx context.Context, obj types.Object) error {
	if obj == nil {
		return nil
	}
	content := map[string]any{
		"specification": obj.GetContent(),
	}
	obj.SetContent(content)
	return nil
}
