// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype

import (
	filev1 "github.com/telekom/controlplane/file/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the FileType catalogue module registration variable.
var Module = &module.TypedModule[*filev1.FileType, *FileTypeData, FileTypeKey]{
	ModuleName: "filetype",
	NewObj:     func() *filev1.FileType { return &filev1.FileType{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[FileTypeKey, *FileTypeData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
		)
	},
}
