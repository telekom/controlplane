// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package contextutil

import (
	"context"
	"time"
)

type contextKey string

var (
	envKey           contextKey = "env"
	reconcileHintKey contextKey = "reconcileHint"
)

// ReconcileHint allows handlers to communicate reconciliation preferences
// back to the common controller. The controller injects a pointer into the
// context before calling the handler; the handler may mutate it.
type ReconcileHint struct {
	// RequeueAfter, when non-nil, overrides the default happy-path requeue
	// interval returned by the controller after a successful reconciliation.
	RequeueAfter *time.Duration
}

// WithReconcileHint stores a ReconcileHint pointer in the context.
func WithReconcileHint(ctx context.Context, hint *ReconcileHint) context.Context {
	return context.WithValue(ctx, reconcileHintKey, hint)
}

// ReconcileHintFromContext retrieves the ReconcileHint from the context.
func ReconcileHintFromContext(ctx context.Context) (*ReconcileHint, bool) {
	h, ok := ctx.Value(reconcileHintKey).(*ReconcileHint)
	return h, ok
}

// SetRequeueAfter is a convenience helper that handlers can call to override
// the default happy-path requeue interval. It is a no-op if the context does
// not carry a ReconcileHint (e.g. in unit tests that skip the common controller).
func SetRequeueAfter(ctx context.Context, d time.Duration) {
	if hint, ok := ReconcileHintFromContext(ctx); ok {
		hint.RequeueAfter = &d
	}
}

func EnvFromContext(ctx context.Context) (string, bool) {
	e, ok := ctx.Value(envKey).(string)
	return e, ok
}

func WithEnv(ctx context.Context, e string) context.Context {
	return context.WithValue(ctx, envKey, e)
}

func EnvFromContextOrDie(ctx context.Context) string {
	e, ok := EnvFromContext(ctx)
	if !ok {
		panic("env not found in context")
	}
	return e
}
