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

// ApprovalRequest holds the schema definition for the ApprovalRequest entity.
type ApprovalRequest struct {
	ent.Schema
}

func (ApprovalRequest) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.NamespaceMixin{},
		schemamixin.ApprovalFieldsMixin{},
	}
}

func (ApprovalRequest) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("state").
			NamedValues(
				"Pending", "PENDING",
				"Semigranted", "SEMIGRANTED",
				"Granted", "GRANTED",
				"Rejected", "REJECTED",
			).
			Default("PENDING"),
	}
}

func (ApprovalRequest) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("api_subscription", ApiSubscription.Type).
			Ref("approval_request").
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
	}
}

func (ApprovalRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.MultiOrder(),
		entgql.RelayConnection(),
	}
}
