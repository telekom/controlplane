// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentcard

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the AgentCard catalogue module registration variable. It wires
// the AgentCard translator and repository into the generic pipeline via
// TypedModule.
//
// AgentCard is a Level 2 entity with a required FK dependency on Team.
var Module = &module.TypedModule[*agenticv1.AgentCard, *AgentCardData, AgentCardKey]{
	ModuleName: "agentcard",
	NewObj:     func() *agenticv1.AgentCard { return &agenticv1.AgentCard{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[AgentCardKey, *AgentCardData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
