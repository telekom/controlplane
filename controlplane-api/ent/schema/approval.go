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
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
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
	}
}

func (Approval) Fields() []ent.Field {
	return []ent.Field{
		field.Text("action").
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
		field.Enum("strategy").
			NamedValues(
				"Auto", "AUTO",
				"Simple", "SIMPLE",
				"FourEyes", "FOUR_EYES",
			).
			Default("AUTO"),
		field.JSON("requester", model.RequesterInfo{}).
			Annotations(entgql.Type("RequesterInfo"), entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("decider", model.DeciderInfo{}).
			Annotations(entgql.Type("DeciderInfo"), entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("decisions", []model.Decision{}).
			Default([]model.Decision{}).
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("available_transitions", []model.AvailableTransition{}).
			Default([]model.AvailableTransition{}).
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (Approval) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("api_subscription", ApiSubscription.Type).
			Ref("approval").
			Unique(),
	}
}

func (Approval) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.MultiOrder(),
		entgql.RelayConnection(),
	}
}
