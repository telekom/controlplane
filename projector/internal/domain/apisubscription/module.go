// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the ApiSubscription module registration variable. It wires the
// ApiSubscription translator and repository into the generic pipeline via
// TypedModule.
//
// ApiSubscription is a Level 3 entity with a required FK dependency on
// Application (owner) and an optional FK dependency on ApiExposure (target).
// It uses pre-delete before upsert, maintains dual cache entries, and uses
// a convention-based fallback delete strategy so KeyFromDelete always succeeds.
var Module = &module.TypedModule[*apiv1.ApiSubscription, *APISubscriptionData, APISubscriptionKey]{
	ModuleName: "apisubscription",
	NewObj:     func() *apiv1.ApiSubscription { return &apiv1.ApiSubscription{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[APISubscriptionKey, *APISubscriptionData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
