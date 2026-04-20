// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import "context"

// APIExposureDeps declares the FK resolution interface required by the
// ApiExposure repository. Application is a required dependency — if the
// owner Application is missing, the upsert fails with ErrDependencyMissing.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type APIExposureDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
}
