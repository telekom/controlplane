// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription

import "context"

// AgenticSubscriptionDeps declares the FK resolution interfaces required by
// the AgenticSubscription repository.
//
//   - FindApplicationID: resolves the owner Application FK (required). If the
//     owner Application is missing, the upsert fails with ErrDependencyMissing.
//   - FindAgenticExposureByBasePath: resolves the target AgenticExposure FK
//     (optional). The subscription CR doesn't know the target app/team — only
//     the base path. If the target AgenticExposure is missing, the
//     subscription is stored with a NULL target FK and will be linked on a
//     later resync.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type AgenticSubscriptionDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
	FindAgenticExposureByBasePath(ctx context.Context, basePath string) (int, error)
	// EvictAgenticExposureByBasePath clears a stale cached exposure ID after a
	// FK violation, forcing the next resolve to hit the DB.
	EvictAgenticExposureByBasePath(basePath string)
}
