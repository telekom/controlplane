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

// ApiSubscription holds the schema definition for an API subscription.
type ApiSubscription struct {
	ent.Schema
}

func (ApiSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.MetadataMixin{},
	}
}

func (ApiSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Text("base_path").
			NotEmpty(),
		field.Enum("m2m_auth_method").
			NamedValues(
				"None", "NONE",
				"BasicAuth", "BASIC_AUTH",
				"Oauth2Client", "OAUTH2_CLIENT",
				"ScopesOnly", "SCOPES_ONLY",
			).
			Default("NONE"),
		field.JSON("approved_scopes", []string{}).
			Default([]string{}).
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (ApiSubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("subscribed_apis").
			Required().
			Unique(),
		edge.To("target", ApiExposure.Type).
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("failover_zones", Zone.Type),
		edge.To("approval", Approval.Type).
			Unique(),
		edge.To("approval_requests", ApprovalRequest.Type),
	}
}

func (ApiSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (ApiSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("base_path").Edges("owner").Unique(),
	}
}
