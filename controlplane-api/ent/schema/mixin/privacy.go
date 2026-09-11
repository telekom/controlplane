// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema/mixin"

	"github.com/telekom/controlplane/controlplane-api/internal/rule"
)

// PrivacyMixin adds privacy policy requiring viewer with team access.
// Mutations are always denied since this is a read-only API.
type PrivacyMixin struct {
	mixin.Schema
}

// Policy defines the default privacy policy.
func (PrivacyMixin) Policy() ent.Policy {
	return privacy.Policy{
		Query: privacy.QueryPolicy{
			rule.DenyIfNoViewer(),
			rule.DenyIfNoTeams(),
		},
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysDenyRule(),
		},
	}
}
