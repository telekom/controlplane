// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package runtime provides the core contracts, error sentinels, and generic
// processing pipeline for the projector.
package runtime

import (
	"errors"
	"fmt"
)

// Sentinel errors used across the projection pipeline. Each error triggers a
// distinct reconciler behavior (see ErrorPolicy).
var (
	// ErrSkipSync is returned when a CR lacks the required data to be synced.
	// The reconciler catches this and requeues at the SkipRequeue interval.
	ErrSkipSync = errors.New("CR lacks required data for sync")

	// ErrDependencyMissing is returned when a referenced entity has not yet
	// been synced. The reconciler requeues with DependencyDelay backoff.
	ErrDependencyMissing = errors.New("dependency not yet synced")

	// ErrDeleteKeyLost is returned by KeyFromDelete when lastKnown is nil
	// and the key cannot be derived from conventions alone.
	// The reconciler emits a metric and logs a warning.
	ErrDeleteKeyLost = errors.New("cannot derive delete key without cached object")
)

// WrapDependencyMissing adds entity context to ErrDependencyMissing.
func WrapDependencyMissing(entityType, name string) error {
	return fmt.Errorf("%s %q: %w", entityType, name, ErrDependencyMissing)
}

// IsSkipSync reports whether err (or any error in its chain) matches
// ErrSkipSync.
func IsSkipSync(err error) bool {
	return errors.Is(err, ErrSkipSync)
}

// IsDependencyMissing reports whether err (or any error in its chain)
// matches ErrDependencyMissing.
func IsDependencyMissing(err error) bool {
	return errors.Is(err, ErrDependencyMissing)
}

// IsDeleteKeyLost reports whether err (or any error in its chain) matches
// ErrDeleteKeyLost.
func IsDeleteKeyLost(err error) bool {
	return errors.Is(err, ErrDeleteKeyLost)
}
