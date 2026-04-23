// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	ctrl "sigs.k8s.io/controller-runtime"

	orgv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the Team module registration variable. It wires the Team
// translator and repository into the generic pipeline via TypedModule.
//
// Team is a Level 1 entity with an optional FK to Group. It uses the
// Strong delete strategy and manages nested Member entities.
var Module = &module.TypedModule[*orgv1.Team, *TeamData, TeamKey]{
	ModuleName: "team",
	NewObj:     func() *orgv1.Team { return &orgv1.Team{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[TeamKey, *TeamData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
			ctrl.Log.WithName("team-repository"),
		)
	},
}
