// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package contextutil

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
)

const recorderKey contextKey = "recorder"

func WithRecorder(ctx context.Context, recorder record.EventRecorder) context.Context {
	return context.WithValue(ctx, recorderKey, recorder)
}

func WithEventRecorder(ctx context.Context, recorder events.EventRecorder) context.Context {
	return context.WithValue(ctx, recorderKey, recorder)
}

func RecoderFromContext(ctx context.Context) (record.EventRecorder, bool) {
	r, ok := ctx.Value(recorderKey).(record.EventRecorder)
	if r == nil {
		return nil, false
	}
	return r, ok
}

func EventRecorderFromContext(ctx context.Context) (events.EventRecorder, bool) {
	r, ok := ctx.Value(recorderKey).(events.EventRecorder)
	if r == nil {
		return nil, false
	}
	return r, ok
}

func RecorderFromContextOrDie(ctx context.Context) record.EventRecorder {
	r, ok := RecoderFromContext(ctx)
	if !ok {
		panic("recorder not found in context")
	}
	return r
}

func EventRecorderFromContextOrDiscard(ctx context.Context) events.EventRecorder {
	r, ok := EventRecorderFromContext(ctx)
	if !ok {
		return noop
	}
	return r
}

var noop events.EventRecorder = &NoopRecorder{}

type NoopRecorder struct{}

// Eventf implements [internal.EventRecorder].
func (n *NoopRecorder) Eventf(regarding, related runtime.Object, eventtype, reason, action, note string, args ...any) {
	// no-op
}
