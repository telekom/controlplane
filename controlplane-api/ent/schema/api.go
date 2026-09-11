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

// Api holds the schema definition for a registered API in the catalogue.
type Api struct {
	ent.Schema
}

func (Api) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (Api) Fields() []ent.Field {
	return []ent.Field{
		field.Text("base_path").
			NotEmpty(),
		field.Text("version").
			NotEmpty(),
		field.Text("category").
			Optional(),
		field.JSON("oauth2_scopes", []string{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.Bool("x_vendor").
			Default(false),
		field.Text("specification").
			Optional().
			Annotations(entgql.Skip(entgql.SkipType)),
		field.Bool("active").
			Default(false),
	}
}

func (Api) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Team.Type).
			Ref("apis").
			Required().
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("exposures", ApiExposure.Type).
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (Api) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (Api) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("base_path").Edges("owner").Unique(),
	}
}
