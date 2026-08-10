// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permissionset

import (
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the PermissionSet module registration variable. It wires the
// PermissionSet translator and repository into the generic pipeline via
// TypedModule.
//
// PermissionSet is a Level 3 entity with a required FK dependency on
// Application. It uses a convention-based fallback delete strategy so
// KeyFromDelete always succeeds. Registration is gated behind the
// FeaturePermission flag in bootstrap.go.
var Module = &module.TypedModule[*permissionv1.PermissionSet, *PermissionSetData, PermissionSetKey]{
	ModuleName: "permissionset",
	NewObj:     func() *permissionv1.PermissionSet { return &permissionv1.PermissionSet{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[PermissionSetKey, *PermissionSetData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
