// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"fmt"
	"net/url"

	"github.com/99designs/gqlgen/graphql"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/secrets"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
)

// Resolver is the root resolver for the GraphQL API.
type Resolver struct {
	client               *ent.Client
	services             service.Services
	secrets              *secrets.Resolver
	specificationBaseURL string
}

// NewResolver creates a new root resolver with the given ent client, services,
// secret resolver, and public base URL of the controlplane-api for specification
// URL construction.
func NewResolver(client *ent.Client, services service.Services, secretResolver *secrets.Resolver, specificationBaseURL string) *Resolver {
	return &Resolver{
		client:               client,
		services:             services,
		secrets:              secretResolver,
		specificationBaseURL: specificationBaseURL,
	}
}

// NewSchema creates a graphql executable schema.
func NewSchema(client *ent.Client, services service.Services, secretResolver *secrets.Resolver, specificationBaseURL string) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Resolvers: NewResolver(client, services, secretResolver, specificationBaseURL),
	})
}

// buildSpecificationURL constructs the public download URL
// (<publicBaseURL>/files/v1/<fileId>) served by the controlplane-api file
// download endpoint. The file ID is path-escaped.
// Returns nil if the specification is empty or the public base URL is not configured.
func (r *Resolver) buildSpecificationURL(specification string) (*string, error) {
	if specification == "" || r.specificationBaseURL == "" {
		return nil, nil
	}
	specURL, err := url.JoinPath(r.specificationBaseURL, "files", "v1", url.PathEscape(specification))
	if err != nil {
		return nil, fmt.Errorf("building specification url: %w", err)
	}
	return &specURL, nil
}
