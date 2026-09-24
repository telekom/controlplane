// Copyright 2026 Deutsche Telekom IT GmbH
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

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// Listener holds the schema definition for a Spectre listener.
type Listener struct {
	ent.Schema
}

func (Listener) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.MetadataMixin{},
	}
}

func (Listener) Fields() []ent.Field {
	return []ent.Field{
		field.Text("api_base_path").
			NotEmpty().
			Annotations(entgql.Skip(entgql.SkipType | entgql.SkipWhereInput)),
		field.JSON("request_filter", &model.ListenerFilter{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipType | entgql.SkipWhereInput)),
		field.JSON("response_filter", &model.ListenerFilter{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipType | entgql.SkipWhereInput)),
	}
}

func (Listener) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("application", Application.Type).
			Ref("listeners").
			Required().
			Unique().
			Annotations(entgql.Skip(entgql.SkipType | entgql.SkipWhereInput)),
		edge.From("subscription", ApiSubscription.Type).
			Ref("listeners").
			Required().
			Unique().
			Annotations(entgql.Skip(entgql.SkipType | entgql.SkipWhereInput)),
		edge.From("exposure", ApiExposure.Type).
			Ref("listeners").
			Required().
			Unique().
			Annotations(entgql.Skip(entgql.SkipType | entgql.SkipWhereInput)),
		edge.To("provider_approval", Approval.Type).
			Unique().
			Annotations(entgql.Skip(entgql.SkipType|entgql.SkipWhereInput), entsql.OnDelete(entsql.Cascade)),
		edge.To("consumer_approval", Approval.Type).
			Unique().
			Annotations(entgql.Skip(entgql.SkipType|entgql.SkipWhereInput), entsql.OnDelete(entsql.Cascade)),
		edge.To("approval_requests", ApprovalRequest.Type).
			Annotations(entgql.Skip(entgql.SkipType|entgql.SkipWhereInput), entsql.OnDelete(entsql.Cascade)),
	}
}

func (Listener) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.MultiOrder(),
		entgql.RelayConnection(),
	}
}
