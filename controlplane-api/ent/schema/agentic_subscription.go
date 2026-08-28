// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// AgenticSubscription holds the schema definition for a subscription to an
// exposed MCP server or A2A agent.
type AgenticSubscription struct {
	ent.Schema
}

func (AgenticSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.MetadataMixin{},
	}
}

func (AgenticSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Text("base_path").
			NotEmpty(),
		field.JSON("security", model.AgenticSubscriptionSecurity{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("traffic", model.AgenticSubscriberTraffic{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (AgenticSubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("agentic_subscriptions").
			Required().
			Unique(),
		edge.To("target", AgenticExposure.Type).
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("approval", Approval.Type).
			Unique(),
		edge.To("approval_requests", ApprovalRequest.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (AgenticSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (AgenticSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("base_path").Edges("owner").Unique(),
	}
}
