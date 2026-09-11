// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	appv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the Application module registration variable. It wires the
// Application translator and repository into the generic pipeline via
// TypedModule.
//
// Application is a Level 2 entity with required FK dependencies on both
// Team and Zone. It uses a convention-based fallback delete strategy
// (TeamNameFromNamespace) so KeyFromDelete always succeeds.
var Module = &module.TypedModule[*appv1.Application, *ApplicationData, ApplicationKey]{
	ModuleName: "application",
	NewObj:     func() *appv1.Application { return &appv1.Application{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[ApplicationKey, *ApplicationData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
