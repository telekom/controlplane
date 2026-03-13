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

// StatusMixin adds a ResourceStatus JSON field.
type StatusMixin struct {
	mixin.Schema
}

func (StatusMixin) Fields() []ent.Field {
	return []ent.Field{
		field.JSON("status", model.ResourceStatus{}).
			Default(model.ResourceStatus{Phase: model.ResourceStatusPhaseUnknown}).
			Annotations(entgql.Type("ResourceStatus")),
	}
}
