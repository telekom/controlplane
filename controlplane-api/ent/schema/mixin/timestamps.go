// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mixin

import (
	"time"

	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// TimestampsMixin adds created_at and last_modified_at fields.
type TimestampsMixin struct {
	mixin.Schema
}

func (TimestampsMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entgql.OrderField("CREATED_AT")),
		field.Time("last_modified_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Annotations(entgql.OrderField("LAST_MODIFIED_AT")),
	}
}
