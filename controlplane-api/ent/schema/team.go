// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
)

// Team holds the schema definition for the Team entity.
type Team struct {
	ent.Schema
}

func (Team) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
	}
}

func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.Text("name").
			NotEmpty().
			Unique().
			Annotations(entgql.OrderField("NAME")),
		field.Text("email").
			NotEmpty(),
		field.Enum("category").
			NamedValues(
				"Customer", "CUSTOMER",
				"Infrastructure", "INFRASTRUCTURE",
			).
			Default("CUSTOMER"),
	}
}

func (Team) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group", Group.Type).
			Ref("teams").
			Unique(),
		edge.To("members", Member.Type),
		edge.To("team_environments", TeamEnvironment.Type),
		edge.To("applications", Application.Type).
			Annotations(entgql.RelayConnection()),
	}
}

func (Team) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.MultiOrder(),
		entgql.RelayConnection(),
	}
}
