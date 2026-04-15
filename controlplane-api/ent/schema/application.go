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
)

// Application holds the schema definition for the Application entity.
type Application struct {
	ent.Schema
}

func (Application) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (Application) Fields() []ent.Field {
	return []ent.Field{
		field.Text("name").
			NotEmpty().
			Annotations(entgql.OrderField("NAME")),
		field.Text("client_id").
			Optional().
			Nillable().
			NotEmpty(),
		field.Text("client_secret").
			Optional().
			Nillable().
			NotEmpty(),
		field.Text("issuer_url").
			Optional().
			Nillable(),
	}
}

func (Application) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("zone", Zone.Type).
			Ref("applications").
			Unique().
			Required(),
		edge.From("owner_team", Team.Type).
			Ref("applications").
			Unique().
			Required().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("exposed_apis", ApiExposure.Type).
			Annotations(entgql.RelayConnection()),
		edge.To("subscribed_apis", ApiSubscription.Type).
			Annotations(entgql.RelayConnection()),
	}
}

func (Application) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.MultiOrder(),
		entgql.RelayConnection(),
	}
}

func (Application) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Edges("owner_team").Unique(),
	}
}
