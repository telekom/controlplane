// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// StatusMixin adds status_phase and status_message fields to an entity.
type StatusMixin struct {
	mixin.Schema
}

func (StatusMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("status_phase").
			NamedValues(
				"Ready", "READY",
				"Pending", "PENDING",
				"Error", "ERROR",
				"Unknown", "UNKNOWN",
			).
			Default("UNKNOWN"),
		field.Text("status_message").
			Optional().
			Nillable(),
	}
}
