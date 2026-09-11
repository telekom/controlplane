// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the Zone module registration variable. It wires the Zone
// translator and repository into the generic pipeline via TypedModule.
//
// Zone is a root entity (Level 0) with no FK dependencies and uses the
// Strong delete strategy.
var Module = &module.TypedModule[*adminv1.Zone, *ZoneData, ZoneKey]{
	ModuleName: "zone",
	NewObj:     func() *adminv1.Zone { return &adminv1.Zone{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[ZoneKey, *ZoneData] {
		return NewRepository(deps.EntClient, deps.EdgeCache)
	},
}
