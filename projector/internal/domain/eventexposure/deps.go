// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import "context"

// EventExposureDeps declares the FK resolution interface required by the
// EventExposure repository. Application is a required dependency — if the
// owner Application is missing, the upsert fails with ErrDependencyMissing.
// EventType is an optional dependency — if no active EventType entry exists
// for the type identifier, the FK is left null.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type EventExposureDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
	FindActiveEventTypeID(ctx context.Context, eventType string) (int, error)
}
