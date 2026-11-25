// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the ApiExposure module registration variable. It wires the
// ApiExposure translator and repository into the generic pipeline via
// TypedModule.
//
// ApiExposure is a Level 3 entity with a required FK dependency on
// Application. It uses a convention-based fallback delete strategy so
// KeyFromDelete always succeeds.
var Module = &module.TypedModule[*apiv1.ApiExposure, *APIExposureData, APIExposureKey]{
	ModuleName: "apiexposure",
	NewObj:     func() *apiv1.ApiExposure { return &apiv1.ApiExposure{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[APIExposureKey, *APIExposureData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
