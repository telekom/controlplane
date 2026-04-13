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
	client             *ent.Client
	teamService        service.TeamService
	applicationService service.ApplicationService
}

// NewResolver creates a new root resolver with the given ent client and optional services.
func NewResolver(client *ent.Client, teamService service.TeamService, applicationService service.ApplicationService) *Resolver {
	return &Resolver{client: client, teamService: teamService, applicationService: applicationService}
}

// NewSchema creates a graphql executable schema.
func NewSchema(client *ent.Client, teamService service.TeamService, applicationService service.ApplicationService) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Resolvers: NewResolver(client, teamService, applicationService),
	})
}
