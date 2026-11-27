// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiroadmap

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module registers the apiroadmap projection. It only updates the Api row,
// so a roadmap projected before its Api is retried until the Api exists.
var Module = &module.TypedModule[*apiv1.ApiRoadmap, *Data, Key]{
	ModuleName: "apiroadmap",
	NewObj:     func() *apiv1.ApiRoadmap { return &apiv1.ApiRoadmap{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[Key, *Data] {
		return NewRepository(deps.EntClient)
	},
}
