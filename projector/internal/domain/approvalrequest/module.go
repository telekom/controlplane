// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/projector/internal/module"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// Module is the ApprovalRequest module registration variable. It wires the
// ApprovalRequest translator and repository into the generic pipeline via
// TypedModule.
//
// ApprovalRequest is a Level 4 entity with a required FK dependency on
// ApiSubscription (M2O). It resolves the subscription FK via cache-based
// meta-key lookup and uses namespace+name as the unique conflict key.
var Module = &module.TypedModule[*approvalv1.ApprovalRequest, *ApprovalRequestData, ApprovalRequestKey]{
	ModuleName: "approvalrequest",
	NewObj:     func() *approvalv1.ApprovalRequest { return &approvalv1.ApprovalRequest{} },
	Translator: &Translator{},
	RepoFactory: func(deps module.ModuleDeps) runtime.Repository[ApprovalRequestKey, *ApprovalRequestData] {
		return NewRepository(
			deps.EntClient,
			deps.EdgeCache,
			deps.IDResolver,
		)
	},
}
