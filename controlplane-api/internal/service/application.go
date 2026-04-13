// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
)

// ApplicationService defines operations for managing Application resources.
type ApplicationService interface {
	RotateApplicationSecret(ctx context.Context, input model.RotateApplicationSecretInput) (*model.ApplicationMutationResult, error)
}
