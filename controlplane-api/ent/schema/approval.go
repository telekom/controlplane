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

// Approval holds the schema definition for the Approval entity.
type Approval struct {
	ent.Schema
}

func (Approval) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
		schemamixin.ApprovalFieldsMixin{},
	}
}

func (Approval) Fields() []ent.Field {
	return []ent.Field{
		field.Text("name").
			NotEmpty(),
		field.Enum("state").
			NamedValues(
				"Pending", "PENDING",
				"Semigranted", "SEMIGRANTED",
				"Granted", "GRANTED",
				"Rejected", "REJECTED",
				"Suspended", "SUSPENDED",
				"Expired", "EXPIRED",
			).
			Default("PENDING"),
	}
}

func (Approval) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("namespace", "name").Unique(),
	}
}

func (Approval) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("api_subscription", ApiSubscription.Type).
			Ref("approval").
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (Approval) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.MultiOrder(),
		entgql.RelayConnection(),
	}
}
