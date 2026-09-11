// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import "context"

// EventSubscriptionDeps declares the FK resolution interfaces required by the
// EventSubscription repository.
//
//   - FindApplicationID: resolves the owner Application FK (required). If the
//     owner Application is missing, the upsert fails with ErrDependencyMissing.
//   - FindEventExposureByEventType: resolves the target EventExposure FK
//     (optional). The subscription CR doesn't know the target app/team — only
//     the event type. If the target EventExposure is missing, the subscription
//     is stored with a NULL target FK and will be linked on a later resync.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type EventSubscriptionDeps interface {
	FindApplicationID(ctx context.Context, name, teamName string) (int, error)
	FindEventExposureByEventType(ctx context.Context, eventType string) (int, error)
}
