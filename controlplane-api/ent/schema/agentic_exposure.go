// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// AgenticExposure holds the schema definition for an exposed MCP server or A2A agent.
type AgenticExposure struct {
	ent.Schema
}

func (AgenticExposure) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (AgenticExposure) Fields() []ent.Field {
	return []ent.Field{
		field.Text("base_path").
			NotEmpty(),
		field.Enum("visibility").
			NamedValues(
				"World", "WORLD",
				"Zone", "ZONE",
				"Enterprise", "ENTERPRISE",
			).
			Default("ENTERPRISE"),
		field.Enum("variant").
			NamedValues(
				"Mcp", "MCP",
				"TelecontextMcp", "TELECONTEXTMCP",
				"Agent", "AGENT",
			).
			Default("MCP"),
		field.Bool("active").
			Optional().
			Nillable().
			Default(false),
		field.JSON("upstreams", []model.Upstream{}).
			Default([]model.Upstream{}).
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("approval_config", model.ApprovalConfig{}).
			Default(model.ApprovalConfig{Strategy: "AUTO"}).
			Annotations(entgql.Type("ApprovalConfig"), entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("security", model.AgenticExposureSecurity{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("traffic", model.Traffic{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("transformation", model.AgenticTransformation{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (AgenticExposure) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("agentic_exposures").
			Required().
			Unique(),
		edge.From("mcp_server", McpServer.Type).
			Ref("exposures").
			Unique(),
		edge.From("agent_card", AgentCard.Type).
			Ref("exposures").
			Unique(),
		edge.From("subscriptions", AgenticSubscription.Type).
			Ref("target").
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (AgenticExposure) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (AgenticExposure) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("base_path").Edges("owner").Unique(),
	}
}
