// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package contextutil

import "context"

type contextKey string

var (
	envKey contextKey = "env"
)

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
