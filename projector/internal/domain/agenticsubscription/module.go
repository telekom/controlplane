// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the AgenticSubscription module registration variable. It wires
// the AgenticSubscription translator and repository into the generic
// pipeline via TypedModule.
//
// AgenticSubscription is a Level 3 entity with a required FK dependency on
// Application (owner) and an optional FK dependency on AgenticExposure
// (target). It uses pre-delete before upsert, maintains dual cache entries,
// and uses a convention-based fallback delete strategy so KeyFromDelete
// always succeeds.
var Module = &module.TypedModule[*agenticv1.AgenticSubscription, *AgenticSubscriptionData, AgenticSubscriptionKey]{
	ModuleName: "agenticsubscription",
	NewObj:     func() *agenticv1.AgenticSubscription { return &agenticv1.AgenticSubscription{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[AgenticSubscriptionKey, *AgenticSubscriptionData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
