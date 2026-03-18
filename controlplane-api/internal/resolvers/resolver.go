// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"github.com/99designs/gqlgen/graphql"
	"github.com/telekom/controlplane/controlplane-api/ent"
)

// Resolver is the root resolver for the GraphQL API.
type Resolver struct{ client *ent.Client }

// NewResolver creates a new root resolver with the given ent client.
func NewResolver(client *ent.Client) *Resolver {
	return &Resolver{client: client}
}

// NewSchema creates a graphql executable schema.
func NewSchema(client *ent.Client) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Resolvers: &Resolver{client},
	})
}
