// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"

	commonmiddleware "github.com/telekom/controlplane/common-server/pkg/server/middleware"
)

const TraceEnabledEnvVar = "SECRET_TRACE_ENABLED"

type traceIDKey struct{}

func Enabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(TraceEnabledEnvVar)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if cid, ok := commonmiddleware.CorrelationIDFromContext(ctx); ok {
		return cid
	}
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

func EnsureTraceID(ctx context.Context) (context.Context, string) {
	if existing := TraceID(ctx); existing != "" {
		return ctx, existing
	}
	tid := NewTraceID()
	return WithTraceID(ctx, tid), tid
}

func NewTraceID() string {
	var buf [8]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return "trace-id-unavailable"
	}
	return hex.EncodeToString(buf[:])
}
