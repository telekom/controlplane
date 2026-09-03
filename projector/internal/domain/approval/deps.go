// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import "context"

// ApprovalDeps declares the FK resolution interfaces required by the
// Approval repository.
//
//   - FindAPISubscriptionByMeta: resolves the parent ApiSubscription FK
//     by k8s namespace + name.
//   - FindEventSubscriptionByMeta: resolves the parent EventSubscription FK
//     by k8s namespace + name.
//   - FindFileSubscriptionByMeta: resolves the parent FileSubscription FK
//     by k8s namespace + name.
//   - EvictAPISubscription: evicts a stale cached ApiSubscription ID.
//   - EvictEventSubscription: evicts a stale cached EventSubscription ID.
//   - EvictFileSubscription: evicts a stale cached FileSubscription ID.
//
// The repository uses one of these depending on the target kind
// (ApiSubscription, EventSubscription, or FileSubscription).
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type ApprovalDeps interface {
	FindAPISubscriptionByMeta(ctx context.Context, namespace, name string) (int, error)
	FindEventSubscriptionByMeta(ctx context.Context, namespace, name string) (int, error)
	FindFileSubscriptionByMeta(ctx context.Context, namespace, name string) (int, error)
	EvictAPISubscription(namespace, name string)
	EvictEventSubscription(namespace, name string)
	EvictFileSubscription(namespace, name string)
}
