// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import "context"

// APIExposureDeps declares the FK resolution interface required by the
// ApiExposure repository. Application is a required dependency — if the
// owner Application is missing, the upsert fails with ErrDependencyMissing.
// Api is an optional dependency — if no active Api entry exists for the
// base path, the FK is left null.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type APIExposureDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
	FindActiveApiID(ctx context.Context, basePath string) (int, error)
}
