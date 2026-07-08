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

// EventSubscription holds the schema definition for an event subscription.
type EventSubscription struct {
	ent.Schema
}

func (EventSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.MetadataMixin{},
	}
}

func (EventSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Text("event_type").
			NotEmpty(),
		field.Enum("delivery_type").
			NamedValues(
				"Callback", "CALLBACK",
				"ServerSentEvent", "SERVER_SENT_EVENT",
			).
			Default("CALLBACK"),
		field.JSON("trigger", &model.EventTrigger{}).
			Optional().
			Default(&model.EventTrigger{}).
			Annotations(entgql.Type("EventTrigger"), entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("delivery", model.EventDelivery{}).
			Default(model.EventDelivery{}).
			Annotations(entgql.Type("EventDelivery"), entgql.Skip(entgql.SkipWhereInput)),
		field.JSON("scopes", []string{}).
			Optional().
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
		field.Text("callback_url").
			Optional().
			Nillable(),
		field.Text("gateway_consumer_url").
			Optional().
			Nillable(),
	}
}

func (EventSubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("subscribed_events").
			Required().
			Unique(),
		edge.To("target", EventExposure.Type).
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.To("approval", Approval.Type).
			Unique(),
		edge.To("approval_requests", ApprovalRequest.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (EventSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (EventSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_type").Edges("owner").Unique(),
	}
}
