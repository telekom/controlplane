// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventtype

import (
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the EventType catalogue module registration variable. It wires the
// EventType translator and repository into the generic pipeline via TypedModule.
//
// EventType is a Level 2 entity with a required FK dependency on Team.
// Registered behind the FeaturePubSub feature flag.
var Module = &module.TypedModule[*eventv1.EventType, *EventTypeData, EventTypeKey]{
	ModuleName: "eventtype",
	NewObj:     func() *eventv1.EventType { return &eventv1.EventType{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[EventTypeKey, *EventTypeData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
