// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
)

// Member holds the schema definition for the Member entity.
type Member struct {
	ent.Schema
}

func (Member) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.EnvironmentMixin{},
	}
}

func (Member) Fields() []ent.Field {
	return []ent.Field{
		field.Text("name").
			NotEmpty(),
		field.Text("email").
			NotEmpty(),
	}
}

func (Member) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("team", Team.Type).
			Ref("members").
			Unique(),
	}
}

func (Member) Indexes() []ent.Index {
	return []ent.Index{
		// Composite unique: a member email must be unique within a team.
		index.Fields("email").Edges("team").Unique(),
		// Lookup index: find all teams a user (by email) belongs to.
		index.Fields("email"),
	}
}
