// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package contextutil

import (
	"context"

	"k8s.io/client-go/tools/record"
)

const recorderKey contextKey = "recorder"

func WithRecorder(ctx context.Context, recorder record.EventRecorder) context.Context {
	return context.WithValue(ctx, recorderKey, recorder)
}

func RecoderFromContext(ctx context.Context) (record.EventRecorder, bool) {
	r, ok := ctx.Value(recorderKey).(record.EventRecorder)
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
