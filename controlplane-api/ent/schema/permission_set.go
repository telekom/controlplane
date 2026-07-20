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

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// PermissionSet holds the schema definition for the PermissionSet entity.
// It represents the role/resource/action grants synced from the
// permission.cp.ei.telekom.de PermissionSet CRD, owned 1:1 by an Application.
type PermissionSet struct {
	ent.Schema
}

func (PermissionSet) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
	}
}

func (PermissionSet) Fields() []ent.Field {
	return []ent.Field{
		field.JSON("permissions", []model.Permission{}).
			Optional().
			Default([]model.Permission{}).
			Annotations(entgql.Type("[Permission]"), entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (PermissionSet) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner_application", Application.Type).
			Ref("permission_set").
			Unique().
			Required().
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (PermissionSet) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}
