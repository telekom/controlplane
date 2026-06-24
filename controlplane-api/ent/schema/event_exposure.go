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
	"entgo.io/ent/schema/index"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// EventExposure holds the schema definition for an exposed event.
type EventExposure struct {
	ent.Schema
}

func (EventExposure) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (EventExposure) Fields() []ent.Field {
	return []ent.Field{
		field.Text("event_type").
			NotEmpty(),
		field.Enum("visibility").
			NamedValues(
				"World", "WORLD",
				"Zone", "ZONE",
				"Enterprise", "ENTERPRISE",
			).
			Default("ENTERPRISE"),
		field.Bool("active").
			Optional().
			Nillable().
			Default(false),
		field.JSON("event_scopes", []model.EventScope{}).
			Default([]model.EventScope{}).
			Annotations(entgql.Type("[EventScope]"), entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("approval_config", model.ApprovalConfig{}).
			Default(model.ApprovalConfig{Strategy: "AUTO"}).
			Annotations(entgql.Type("ApprovalConfig"), entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (EventExposure) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("exposed_events").
			Required().
			Unique(),
		edge.From("event_type_def", EventType.Type).
			Ref("exposures").
			Unique(),
		edge.From("subscriptions", EventSubscription.Type).
			Ref("target").
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (EventExposure) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (EventExposure) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_type").Edges("owner").Unique(),
	}
}
