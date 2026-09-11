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
)

// EventType holds the schema definition for a registered event type in the catalogue.
type EventType struct {
	ent.Schema
}

func (EventType) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (EventType) Fields() []ent.Field {
	return []ent.Field{
		field.Text("event_type").
			NotEmpty(),
		field.Text("version").
			NotEmpty(),
		field.Text("description").
			Optional(),
		field.Text("specification").
			Optional().
			Annotations(entgql.Skip(entgql.SkipType)),
		field.Bool("active").
			Default(false),
	}
}

func (EventType) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Team.Type).
			Ref("event_types").
			Required().
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("exposures", EventExposure.Type).
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (EventType) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (EventType) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_type").Edges("owner").Unique(),
	}
}
