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

// Zone holds the schema definition for the Zone entity.
type Zone struct {
	ent.Schema
}

func (Zone) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
	}
}

func (Zone) Fields() []ent.Field {
	return []ent.Field{
		field.Text("name").
			NotEmpty().
			Unique(),
		field.Text("gateway_url").
			Optional().
			Nillable(),
		field.Enum("visibility").
			NamedValues(
				"World", "WORLD",
				"Enterprise", "ENTERPRISE",
			).
			Default("ENTERPRISE"),
	}
}

func (Zone) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("applications", Application.Type),
	}
}

func (Zone) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
	}
}
