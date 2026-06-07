// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/go-logr/logr"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

// LogMutationUser returns a gqlgen AroundOperations middleware that logs the
// requesting user's name and email for every GraphQL mutation.
// It must be registered after ViewerFromBusinessContext so that the Viewer is
// available in the context.
func LogMutationUser() graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc.Operation != nil && oc.Operation.Operation == ast.Mutation {
			log := logr.FromContextOrDiscard(ctx)
			operationName := oc.OperationName
			var userName, userEmail string
			if v := viewer.FromContext(ctx); v != nil {
				userName = v.UserName
				userEmail = v.UserEmail
			}
			log.Info("Mutation requested",
				"operation", operationName,
				"userName", userName,
				"userEmail", userEmail,
			)
		}
		return next(ctx)
	}
}
