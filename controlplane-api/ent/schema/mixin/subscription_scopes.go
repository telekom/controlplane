// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mixin

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// SubscriptionScopesMixin separates requested scopes from the last provisioned scope set.
type SubscriptionScopesMixin struct {
	mixin.Schema
}

func (SubscriptionScopesMixin) Fields() []ent.Field {
	return []ent.Field{
		field.JSON("requested_scopes", []string{}).
			Optional().
			Comment("Latest scopes requested in the subscription specification.").
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("active_scopes", []string{}).
			Optional().
			Comment("Last approved scope set successfully written to downstream Kubernetes resources; does not imply data-plane readiness.").
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
	}
}
