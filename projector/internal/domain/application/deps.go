// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import "context"

// ApplicationDeps declares the FK resolution interface required by the
// Application repository. Both Team and Zone are required dependencies —
// if either is missing, the upsert fails with ErrDependencyMissing.
//
// If the target PermissionsSetID is missing, the application is stored
//
//	with a NULL target FK and will be linked on a later resync.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type ApplicationDeps interface {
	FindTeamID(ctx context.Context, name string) (int, error)
	FindZoneID(ctx context.Context, name string) (int, error)
	FindPermissionSetIDByApplicationOwner(ctx context.Context, appName, teamName string) (int, error)
}
