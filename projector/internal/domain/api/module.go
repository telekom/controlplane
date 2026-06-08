// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the Api catalogue module registration variable. It wires the
// Api translator and repository into the generic pipeline via TypedModule.
//
// Api is a Level 2 entity with a required FK dependency on Team.
var Module = &module.TypedModule[*apiv1.Api, *ApiData, ApiKey]{
	ModuleName: "api",
	NewObj:     func() *apiv1.Api { return &apiv1.Api{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[ApiKey, *ApiData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
