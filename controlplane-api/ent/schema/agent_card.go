// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package schema //nolint:dupl // structurally mirrors McpServer by design (sibling catalogue entity); ent's one-schema-per-entity convention makes further extraction impractical.

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
)

// AgentCard holds the schema definition for a registered A2A agent in the catalogue.
type AgentCard struct {
	ent.Schema
}

func (AgentCard) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (AgentCard) Fields() []ent.Field {
	return []ent.Field{
		field.Text("base_path").
			NotEmpty(),
		field.Text("version").
			NotEmpty(),
		field.Text("name").
			NotEmpty().
			Comment("Resource name of the agent, derived from its base path."),
		field.Text("display_name").
			Default("").
			Comment("Human-readable name of the agent: the title from its specification, " +
				"or the resource name if the specification has no title."),
		field.Text("description").
			Optional(),
		field.Text("specification").
			Optional().
			Annotations(entgql.Skip(entgql.SkipType)),
		field.Text("category").
			Optional(),
		field.JSON("oauth2_scopes", []string{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.Bool("active").
			Default(false).
			Comment("Only the oldest entry for a base path is active. Entries of other teams with the same base path " +
				"are inactive. Filter on active to get one agent per base path."),
	}
}

func (AgentCard) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Team.Type).
			Ref("agent_cards").
			Required().
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("exposures", AgenticExposure.Type).
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (AgentCard) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (AgentCard) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("base_path").Edges("owner").Unique(),
	}
}
