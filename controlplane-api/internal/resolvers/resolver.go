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
	client      *ent.Client
	teamService service.TeamService
}

// NewResolver creates a new root resolver with the given ent client and optional team service.
func NewResolver(client *ent.Client, teamService service.TeamService) *Resolver {
	return &Resolver{client: client, teamService: teamService}
}

// NewSchema creates a graphql executable schema.
func NewSchema(client *ent.Client, teamService service.TeamService) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Resolvers: NewResolver(client, teamService),
	})
}
