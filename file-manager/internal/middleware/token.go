// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
)

// ContextBearerTokenKey is the key used to store the bearer token in the context
type contextKey string

const BearerTokenKey contextKey = "bearer-token"

// ExtractBearerTokenFromContext extracts the bearer token from the context
// The token is expected to be stored in the context with the BearerTokenKey
func ExtractBearerTokenFromContext(ctx context.Context) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Check if token exists in context
	tokenVal := ctx.Value(BearerTokenKey)
	if tokenVal == nil {
		log.V(1).Info("No bearer token found in context")
		return "", errors.New("no bearer token found in context")
	}

	// Convert to string
	token, ok := tokenVal.(string)
	if !ok {
		log.Error(nil, "Bearer token in context is not a string")
		return "", errors.New("bearer token in context is not a string")
	}

	// Remove "Bearer " prefix if present
	token = strings.TrimPrefix(token, "Bearer ")

	log.V(1).Info("Bearer token extracted from context")
	return token, nil
}

// WithBearerToken adds the bearer token to the context
// The token is stored with the BearerTokenKey
func WithBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, BearerTokenKey, token)
}
