// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package schema //nolint:dupl // structurally mirrors AgentCard by design (sibling catalogue entity); ent's one-schema-per-entity convention makes further extraction impractical.

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
)

// McpServer holds the schema definition for a registered MCP server in the catalogue.
type McpServer struct {
	ent.Schema
}

func (McpServer) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (McpServer) Fields() []ent.Field {
	return []ent.Field{
		field.Text("base_path").
			NotEmpty(),
		field.Text("version").
			NotEmpty(),
		field.Text("name").
			NotEmpty(),
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
			Default(false),
	}
}

func (McpServer) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Team.Type).
			Ref("mcp_servers").
			Required().
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("exposures", AgenticExposure.Type).
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (McpServer) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (McpServer) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("base_path").Edges("owner").Unique(),
	}
}
