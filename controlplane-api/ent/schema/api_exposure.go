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
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
)

// ApiExposure holds the schema definition for an exposed API.
type ApiExposure struct {
	ent.Schema
}

func (ApiExposure) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (ApiExposure) Fields() []ent.Field {
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
		field.Bool("active").
			Optional().
			Nillable().
			Default(false),
		field.JSON("features", []string{}).
			Default([]string{}).
			Annotations(entgql.Type("[ApiExposureFeature!]"), entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("upstreams", []model.Upstream{}).
			Default([]model.Upstream{}).
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("approval_config", model.ApprovalConfig{}).
			Default(model.ApprovalConfig{Strategy: "AUTO"}).
			Annotations(entgql.Type("ApprovalConfig"), entgql.Skip(entgql.SkipWhereInput)),
		field.Text("api_version").
			Optional().
			Nillable(),
	}
}

func (ApiExposure) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("exposed_apis").
			Required().
			Unique(),
		edge.From("subscriptions", ApiSubscription.Type).
			Ref("target").
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (ApiExposure) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (ApiExposure) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("base_path").Edges("owner").Unique(),
	}
}
