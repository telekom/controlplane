// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// ApiDeps declares the FK resolution interface required by the Api repository.
// Team is a required dependency — if the owner Team is missing, the upsert
// fails with ErrDependencyMissing.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type ApiDeps interface {
	FindTeamID(ctx context.Context, name string) (int, error)
}
