// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// EnvironmentMixin adds an environment field for querying convenience.
// The environment is implicit (one logical database per environment),
// but this field provides transparency in the GraphQL schema.
type EnvironmentMixin struct {
	mixin.Schema
}

func (EnvironmentMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Text("environment").
			Optional().
			Nillable(),
	}
}
