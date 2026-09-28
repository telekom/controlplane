// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// APISubscription holds the schema definition for an API subscription.
type APISubscription struct {
	ent.Schema
}

func (APISubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.MetadataMixin{},
	}
}

func (APISubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Text("base_path").
			NotEmpty(),
		field.Enum("M2M_auth_method").
			NamedValues(
				"None", "NONE",
				"BasicAuth", "BASIC_AUTH",
				"OAuth2Client", "OAUTH2_CLIENT",
				"ScopesOnly", "SCOPES_ONLY",
			).
			Default("NONE"),
		field.Text("gateway_url").
			Optional().
			Nillable(),
		field.JSON("security", &model.APISubscriptionSecurity{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("traffic", &model.APISubscriptionTraffic{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (APISubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("subscribed_APIs").
			Required().
			Unique(),
		edge.To("target", APIExposure.Type).
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("failover_zones", Zone.Type),
		edge.To("approval", Approval.Type).
			Unique(),
		edge.To("approval_requests", ApprovalRequest.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (APISubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (APISubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("base_path").Edges("owner").Unique(),
	}
}
