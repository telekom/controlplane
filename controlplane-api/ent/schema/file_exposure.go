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

// FileExposure holds the schema definition for an exposed file type.
type FileExposure struct {
	ent.Schema
}

func (FileExposure) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (FileExposure) Fields() []ent.Field {
	return []ent.Field{
		field.Text("file_type").
			NotEmpty(),
		field.Text("provider").
			Optional().
			Nillable(),
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
		field.Text("zone").
			NotEmpty(),
		field.JSON("sftp_public_keys", []string{}).
			Default([]string{}).
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("approval_config", model.ApprovalConfig{}).
			Default(model.ApprovalConfig{Strategy: "AUTO"}).
			Annotations(entgql.Type("ApprovalConfig"), entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (FileExposure) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("exposed_file_types").
			Required().
			Unique(),
		edge.From("file_type_def", FileType.Type).
			Ref("exposures").
			Unique(),
		edge.From("zone", Zone.Type).
			Ref("file_exposures").
			Required().
			Unique(),
		edge.From("subscriptions", FileSubscription.Type).
			Ref("target").
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (FileExposure) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (FileExposure) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("file_type").Edges("owner").Unique(),
	}
}
