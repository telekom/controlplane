// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the McpServer catalogue module registration variable. It wires
// the McpServer translator and repository into the generic pipeline via
// TypedModule.
//
// McpServer is a Level 2 entity with a required FK dependency on Team.
var Module = &module.TypedModule[*agenticv1.McpServer, *McpServerData, McpServerKey]{
	ModuleName: "mcpserver",
	NewObj:     func() *agenticv1.McpServer { return &agenticv1.McpServer{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[McpServerKey, *McpServerData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
