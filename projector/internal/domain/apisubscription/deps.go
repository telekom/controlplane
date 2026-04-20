// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import "context"

// APISubscriptionDeps declares the FK resolution interfaces required by the
// ApiSubscription repository.
//
//   - FindApplicationID: resolves the owner Application FK (required). If the
//     owner Application is missing, the upsert fails with ErrDependencyMissing.
//   - FindAPIExposureByBasePath: resolves the target ApiExposure FK (optional).
//     The subscription CR doesn't know the target app/team — only the base
//     path. If the target ApiExposure is missing, the subscription is stored
//     with a NULL target FK and will be linked on a later resync.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type APISubscriptionDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
	FindAPIExposureByBasePath(ctx context.Context, basePath string) (int, error)
}
