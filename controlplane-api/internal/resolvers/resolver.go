// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"github.com/99designs/gqlgen/graphql"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
)

// Resolver is the root resolver for the GraphQL API.
type Resolver struct {
	client   *ent.Client
	services service.Services
}

// NewResolver creates a new root resolver with the given ent client and services.
func NewResolver(client *ent.Client, services ...service.Services) *Resolver {
	r := &Resolver{client: client}
	if len(services) > 0 {
		r.services = services[0]
	}
	return r
}

// NewSchema creates a graphql executable schema.
func NewSchema(client *ent.Client, services service.Services) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Resolvers: NewResolver(client, services),
	})
}
