// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package group

import (
	orgv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the Group module registration variable. It wires the Group
// translator and repository into the generic pipeline via TypedModule.
//
// Group is a root entity (Level 0) with no FK dependencies and uses the
// Strong delete strategy.
var Module = &module.TypedModule[*orgv1.Group, *GroupData, GroupKey]{
	ModuleName: "group",
	NewObj:     func() *orgv1.Group { return &orgv1.Group{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[GroupKey, *GroupData] {
		return NewRepository(deps.EntClient, deps.EdgeCache)
	},
}
