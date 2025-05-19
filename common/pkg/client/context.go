// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"github.com/pkg/errors"
)

type contextKey string

var (
	clientKey            contextKey = "client"
	ErrNoClientInContext            = errors.New("no client in context")
)

func ClientFromContext(ctx context.Context) (JanitorClient, bool) {
	c, ok := ctx.Value(clientKey).(JanitorClient)
	return c, ok
}

func WithClient(ctx context.Context, c JanitorClient) context.Context {
	return context.WithValue(ctx, clientKey, c)
}

func ClientFromContextOrDie(ctx context.Context) JanitorClient {
	c, ok := ClientFromContext(ctx)
	if !ok {
		panic(ErrNoClientInContext)
	}
	return c
}
