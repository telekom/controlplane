// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mixin

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// NamespaceMixin adds a namespace field following Kubernetes naming conventions.
type NamespaceMixin struct {
	mixin.Schema
}

func (NamespaceMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Text("namespace").
			Optional().
			Nillable(),
	}
}
