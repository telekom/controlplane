// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import "context"

// ApprovalRequestDeps declares the FK resolution interfaces required by the
// ApprovalRequest repository.
//
//   - FindAPISubscriptionByMeta: resolves the parent ApiSubscription FK
//     by k8s namespace + name.
//   - FindEventSubscriptionByMeta: resolves the parent EventSubscription FK
//     by k8s namespace + name.
//   - EvictAPISubscription: evicts a stale cached ApiSubscription ID.
//   - EvictEventSubscription: evicts a stale cached EventSubscription ID.
//
// The repository uses one or the other depending on the target kind
// (ApiSubscription vs EventSubscription).
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type ApprovalRequestDeps interface {
	FindAPISubscriptionByMeta(ctx context.Context, namespace, name string) (int, error)
	FindEventSubscriptionByMeta(ctx context.Context, namespace, name string) (int, error)
	EvictAPISubscription(namespace, name string)
	EvictEventSubscription(namespace, name string)
}
