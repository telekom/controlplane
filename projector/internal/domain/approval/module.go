// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the Approval module registration variable. It wires the Approval
// translator and repository into the generic pipeline via TypedModule.
//
// Approval is a Level 4 entity with a required FK dependency on
// ApiSubscription. It resolves the subscription FK via cache-based meta-key
// lookup and uses namespace+name as the unique conflict key.
var Module = &module.TypedModule[*approvalv1.Approval, *ApprovalData, ApprovalKey]{
	ModuleName: "approval",
	NewObj:     func() *approvalv1.Approval { return &approvalv1.Approval{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[ApprovalKey, *ApprovalData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
