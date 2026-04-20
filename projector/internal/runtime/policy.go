// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"time"

	"github.com/telekom/controlplane/projector/internal/config"
)

// ErrorPolicy configures requeue behavior per error class.
// The reconciler uses these values to decide how to handle classified errors.
type ErrorPolicy struct {
	// SkipRequeue is the requeue interval for ErrSkipSync.
	// Set to 0 to disable requeueing for skipped CRs.
	SkipRequeue time.Duration

	// DependencyDelay is the base requeue delay for ErrDependencyMissing.
	// Kept short to allow fast convergence when dependencies are synced.
	DependencyDelay time.Duration

	// DependencyJitter is the maximum random jitter added to
	// DependencyDelay. The actual requeue delay is:
	//   DependencyDelay + random(0, DependencyJitter)
	// Set to 0 to disable jitter.
	DependencyJitter time.Duration

	// PeriodicResync is the success requeue interval. Set to 0 for
	// event-driven operation (no periodic requeue after success).
	PeriodicResync time.Duration
}

// DefaultErrorPolicy returns production defaults for the error policy.
// These defaults are used when no Config is available (e.g. in tests).
func DefaultErrorPolicy() ErrorPolicy {
	return ErrorPolicy{
		SkipRequeue:      5 * time.Minute,
		DependencyDelay:  1 * time.Second,
		DependencyJitter: 0,
		PeriodicResync:   5 * time.Minute,
	}
}

// NewErrorPolicyFromConfig constructs an ErrorPolicy from the operator config.
func NewErrorPolicyFromConfig(cfg *config.Config) ErrorPolicy {
	return ErrorPolicy{
		SkipRequeue:      cfg.SkipRequeue,
		DependencyDelay:  cfg.DependencyDelay,
		DependencyJitter: cfg.DependencyDelayJitter,
		PeriodicResync:   cfg.PeriodicResync,
	}
}
