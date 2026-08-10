// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import (
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the FileSubscription module registration variable.
var Module = &module.TypedModule[*filev1.FileSubscription, *FileSubscriptionData, FileSubscriptionKey]{
	ModuleName: "filesubscription",
	NewObj:     func() *filev1.FileSubscription { return &filev1.FileSubscription{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[FileSubscriptionKey, *FileSubscriptionData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
