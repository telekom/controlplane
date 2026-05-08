// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"context"
	"errors"

	"entgo.io/ent/privacy"
	"github.com/99designs/gqlgen/graphql"
	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/vektah/gqlparser/v2/gqlerror"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrorPresenter maps internal errors to stable GraphQL error responses.
func ErrorPresenter(ctx context.Context, err error) *gqlerror.Error {
	if err == nil {
		return nil
	}

	var gqlErr *gqlerror.Error
	if errors.As(err, &gqlErr) {
		return graphql.DefaultErrorPresenter(ctx, err)
	}

	presented := graphql.DefaultErrorPresenter(ctx, err)
	if presented == nil {
		return nil
	}

	switch {
	case ent.IsNotFound(err), apierrors.IsNotFound(err):
		presented.Message = "resource not found"
		setGraphQLErrorCode(presented, "NOT_FOUND")
	case errors.Is(err, privacy.Deny), apierrors.IsForbidden(err):
		presented.Message = "forbidden"
		setGraphQLErrorCode(presented, "FORBIDDEN")
	case ent.IsValidationError(err), apierrors.IsInvalid(err):
		presented.Message = "validation failed"
		setGraphQLErrorCode(presented, "VALIDATION_FAILED")
	case apierrors.IsConflict(err), apierrors.IsAlreadyExists(err):
		presented.Message = "conflict"
		setGraphQLErrorCode(presented, "CONFLICT")
	case isHttpError(err):
		logr.FromContextOrDiscard(ctx).Error(err, "External service error in GraphQL resolver")
		presented.Message = "internal error while processing request"
		setGraphQLErrorCode(presented, "INTERNAL")
	default:
		logr.FromContextOrDiscard(ctx).Error(err, "Unhandled GraphQL resolver error")
		presented.Message = "internal error while processing request"
		setGraphQLErrorCode(presented, "INTERNAL")
	}

	return presented
}

func setGraphQLErrorCode(err *gqlerror.Error, code string) {
	if err.Extensions == nil {
		err.Extensions = map[string]any{}
	}
	err.Extensions["code"] = code
}

// isHttpError returns true if the error chain contains a *client.HttpError.
func isHttpError(err error) bool {
	var httpErr *client.HttpError
	return errors.As(err, &httpErr)
}
