// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apichangelog

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module registers the ApiChangelog projection. It only updates the Api row,
// so a changelog projected before its Api is retried until the Api exists.
var Module = &module.TypedModule[*apiv1.ApiChangelog, *Data, Key]{
	ModuleName: "apichangelog",
	NewObj:     func() *apiv1.ApiChangelog { return &apiv1.ApiChangelog{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[Key, *Data] {
		return NewRepository(deps.EntClient)
	},
}
