// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permissionset

import "context"

// PermissionSetDeps declares the FK resolution interface required by the
// PermissionSet repository. Application is a required dependency — if the
// owner Application is missing, the upsert fails with ErrDependencyMissing.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type PermissionSetDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
}
