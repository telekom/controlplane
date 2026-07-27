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
			NotEmpty().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.Text("rotated_client_secret").
			Optional().
			Nillable().
			NotEmpty().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.Time("rotated_expires_at").
			Optional().
			Nillable(),
		field.Time("current_expires_at").
			Optional().
			Nillable(),
		field.Enum("secret_rotation_phase").
			Values("DONE", "ROTATING", "GRACE_PERIOD_ACTIVE", "GRACE_PERIOD_EXPIRING", "FAILED").
			Default("DONE"),
		field.Text("secret_rotation_message").
			Optional().
			Nillable(),
		field.JSON("external_ids", []model.ExternalId{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("ip_restrictions", model.IpRestrictions{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
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
		edge.To("exposed_events", EventExposure.Type).
			Annotations(entgql.RelayConnection()),
		edge.To("subscribed_events", EventSubscription.Type).
			Annotations(entgql.RelayConnection()),
		edge.To("permission_set", PermissionSet.Type).
			Unique(),
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
