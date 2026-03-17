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

// TeamEnvironment holds the schema definition for the TeamEnvironment entity.
// It represents the relationship between a Team and an Environment,
// including per-environment metadata like the rover token reference.
type TeamEnvironment struct {
	ent.Schema
}

func (TeamEnvironment) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
	}
}

func (TeamEnvironment) Fields() []ent.Field {
	return []ent.Field{
		field.Text("rover_token_ref").
			Optional().
			Nillable(),
	}
}

func (TeamEnvironment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("team", Team.Type).
			Ref("team_environments").
			Unique().
			Required(),
		edge.From("environment", Environment.Type).
			Ref("team_environments").
			Unique().
			Required(),
	}
}

func (TeamEnvironment) Indexes() []ent.Index {
	return []ent.Index{
		index.Edges("team", "environment").Unique(),
	}
}
