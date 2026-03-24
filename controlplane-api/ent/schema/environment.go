// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
)

// Environment holds the schema definition for the Environment entity.
type Environment struct {
	ent.Schema
}

func (Environment) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
	}
}

func (Environment) Fields() []ent.Field {
	return []ent.Field{
		field.Text("name").
			NotEmpty().
			Unique(),
	}
}

func (Environment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("team_environments", TeamEnvironment.Type),
	}
}
