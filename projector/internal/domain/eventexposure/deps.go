// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import "context"

// EventExposureDeps declares the FK resolution interface required by the
// EventExposure repository. Application is a required dependency — if the
// owner Application is missing, the upsert fails with ErrDependencyMissing.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type EventExposureDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
}
