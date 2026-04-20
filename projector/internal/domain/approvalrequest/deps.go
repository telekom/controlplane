// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import "context"

// ApprovalRequestDeps declares the FK resolution interfaces required by the
// ApprovalRequest repository.
//
//   - FindAPISubscriptionByMeta: resolves the parent ApiSubscription FK
//     (required) by k8s namespace + name. If the ApiSubscription is missing,
//     the upsert fails with ErrDependencyMissing and the reconciler requeues.
//
// Satisfied by *infrastructure.IDResolver at wiring time.
type ApprovalRequestDeps interface {
	FindAPISubscriptionByMeta(ctx context.Context, namespace, name string) (int, error)
}
