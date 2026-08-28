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
	"entgo.io/ent/schema/index"

	schemamixin "github.com/telekom/controlplane/controlplane-api/ent/schema/mixin"
)

// FileSubscription holds the schema definition for a file type subscription.
type FileSubscription struct {
	ent.Schema
}

func (FileSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		schemamixin.PrivacyMixin{},
		schemamixin.TimestampsMixin{},
		schemamixin.StatusMixin{},
		schemamixin.EnvironmentMixin{},
		schemamixin.MetadataMixin{},
	}
}

func (FileSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Text("file_type").
			NotEmpty(),
		field.Text("zone_name").
			NotEmpty(),
		field.JSON("sftp_public_keys", []string{}).
			Default([]string{}).
			Annotations(entgql.Skip(entgql.SkipWhereInput)),
	}
}

func (FileSubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Application.Type).
			Ref("subscribed_file_types").
			Required().
			Unique(),
		edge.From("file_type_def", FileType.Type).
			Ref("subscriptions").
			Unique(),
		edge.To("target", FileExposure.Type).
			Unique().
			Annotations(entgql.Skip(entgql.SkipType)),
		edge.From("zone", Zone.Type).
			Ref("file_subscriptions").
			Required().
			Unique(),
		edge.To("approval", Approval.Type).
			Unique(),
		edge.To("approval_requests", ApprovalRequest.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}

}

func (FileSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.QueryField(),
		entgql.RelayConnection(),
	}
}

func (FileSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("file_type").Edges("owner").Unique(),
	}
}
