// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mixin

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"

	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
)

// ApprovalFieldsMixin provides the shared fields for Approval and ApprovalRequest entities.
type ApprovalFieldsMixin struct {
	mixin.Schema
}

func (ApprovalFieldsMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Text("action").
			NotEmpty(),
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
