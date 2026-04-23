// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Translator maps a Kubernetes object to a typed domain payload and identity key.
// Each resource module provides its own Translator implementation.
//
// Type parameters:
//   - T: Kubernetes object type (must implement client.Object)
//   - D: typed domain payload (DTO) produced by Translate, consumed by Repository
//   - K: typed identity key used for delete and lookup
type Translator[T client.Object, D any, K any] interface {
	// Translate converts a live K8s object to a domain DTO.
	// Must not return ErrSkipSync — use ShouldSkip for pre-checks.
	Translate(ctx context.Context, obj T) (D, error)

	// ShouldSkip returns true + reason if the object should not be synced.
	// Called before Translate. Cheap check, no transformation performed.
	ShouldSkip(obj T) (skip bool, reason string)

	// KeyFromObject derives the identity key from a live object.
	// Infallible — a live object always has enough data.
	KeyFromObject(obj T) K

	// KeyFromDelete derives the identity key for a delete operation.
	// lastKnown may be nil (cache miss). Returns ErrDeleteKeyLost if key
	// cannot be derived without lastKnown (LastKnownRequired strategy).
	KeyFromDelete(req types.NamespacedName, lastKnown T) (K, error)
}

// Repository performs typed persistence operations for a single entity type.
// Complex internal operations (transactions, cascade deletes, member sync) are
// the responsibility of the concrete implementation.
//
// Type parameters:
//   - K: typed identity key
//   - D: typed domain payload (DTO)
type Repository[K any, D any] interface {
	Upsert(ctx context.Context, data D) error
	Delete(ctx context.Context, key K) error
}

// SyncProcessor defines the operations that the reconciler depends on.
// The generic Processor[T, D, K] implements this interface, providing type
// erasure at the reconciler boundary.
type SyncProcessor[T client.Object] interface {
	Upsert(ctx context.Context, obj T) error
	Delete(ctx context.Context, req types.NamespacedName, lastKnown T) error
}
