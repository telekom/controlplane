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

// Group holds the schema definition for the Group entity.
type Group struct {
	ent.Schema
}

func (Group) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.Text("name").
			NotEmpty().
			Unique(),
		field.Text("display_name").
			NotEmpty(),
		field.Text("description").
			Default(""),
	}
}

func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("teams", Team.Type),
	}
}
