// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the EventSubscription module registration variable. It wires the
// EventSubscription translator and repository into the generic pipeline via
// TypedModule.
//
// EventSubscription is a Level 3 entity with a required FK dependency on
// Application (owner) and an optional FK dependency on EventExposure (target).
var Module = &module.TypedModule[*eventv1.EventSubscription, *EventSubscriptionData, EventSubscriptionKey]{
	ModuleName: "eventsubscription",
	NewObj:     func() *eventv1.EventSubscription { return &eventv1.EventSubscription{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[EventSubscriptionKey, *EventSubscriptionData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
