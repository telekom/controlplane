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

// FileType holds the schema definition for a registered file type in the catalogue.
type FileType struct {
	ent.Schema
}

func (FileType) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (FileType) Fields() []ent.Field {
	return []ent.Field{
		field.Text("file_type").
			NotEmpty(),
		field.Text("description").
			Optional(),
		field.Text("variant").
			Optional().
			Nillable(),
		field.Bool("active").
			Default(false),
		field.Text("sftp_instance_name").
			Optional().
			Nillable(),
		field.Text("sftp_instance_namespace").
			Optional().
			Nillable(),
	}
}

func (FileType) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("exposures", FileExposure.Type).
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("subscriptions", FileSubscription.Type).
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (FileType) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (FileType) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("file_type").Unique(),
	}
}
