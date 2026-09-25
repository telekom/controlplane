// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure

import (
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the FileExposure module registration variable.
var Module = &module.TypedModule[*filev1.FileExposure, *FileExposureData, FileExposureKey]{
	ModuleName: "fileexposure",
	NewObj:     func() *filev1.FileExposure { return &filev1.FileExposure{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[FileExposureKey, *FileExposureData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
