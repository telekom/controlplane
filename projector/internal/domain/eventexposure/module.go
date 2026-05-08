// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import (
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the EventExposure module registration variable. It wires the
// EventExposure translator and repository into the generic pipeline via
// TypedModule.
//
// EventExposure is a Level 3 entity with a required FK dependency on
// Application. It uses a convention-based fallback delete strategy so
// KeyFromDelete always succeeds.
var Module = &module.TypedModule[*eventv1.EventExposure, *EventExposureData, EventExposureKey]{
	ModuleName: "eventexposure",
	NewObj:     func() *eventv1.EventExposure { return &eventv1.EventExposure{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[EventExposureKey, *EventExposureData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
