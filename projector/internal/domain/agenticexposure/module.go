// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the AgenticExposure module registration variable. It wires the
// AgenticExposure translator and repository into the generic pipeline via
// TypedModule.
//
// AgenticExposure is a Level 3 entity with a required FK dependency on
// Application, and mutually exclusive optional FK dependencies on
// McpServer/AgentCard (selected by Variant).
var Module = &module.TypedModule[*agenticv1.AgenticExposure, *AgenticExposureData, AgenticExposureKey]{
	ModuleName: "agenticexposure",
	NewObj:     func() *agenticv1.AgenticExposure { return &agenticv1.AgenticExposure{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[AgenticExposureKey, *AgenticExposureData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
