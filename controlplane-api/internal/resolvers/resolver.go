// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"fmt"

	"github.com/99designs/gqlgen/graphql"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/secrets"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
)

// Resolver is the root resolver for the GraphQL API.
type Resolver struct {
	client             *ent.Client
	services           service.Services
	secrets            *secrets.Resolver
	fileManagerBaseURL string
}

// NewResolver creates a new root resolver with the given ent client, services,
// secret resolver, and file-manager base URL for specification URL construction.
func NewResolver(client *ent.Client, services service.Services, secretResolver *secrets.Resolver, fileManagerBaseURL string) *Resolver {
	return &Resolver{
		client:             client,
		services:           services,
		secrets:            secretResolver,
		fileManagerBaseURL: fileManagerBaseURL,
	}
}

// NewSchema creates a graphql executable schema.
func NewSchema(client *ent.Client, services service.Services, secretResolver *secrets.Resolver, fileManagerBaseURL string) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Resolvers: NewResolver(client, services, secretResolver, fileManagerBaseURL),
	})
}

// buildSpecificationURL constructs a download URL for a file-manager file ID.
// Returns nil if the specification is empty or the file-manager base URL is not configured.
func (r *Resolver) buildSpecificationURL(specification string) (*string, error) {
	if specification == "" || r.fileManagerBaseURL == "" {
		return nil, nil
	}
	url := fmt.Sprintf("%s/files/%s", r.fileManagerBaseURL, specification)
	return &url, nil
}
