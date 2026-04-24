// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package reminder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

func d(h int) metav1.Duration {
	return metav1.Duration{Duration: time.Duration(h) * time.Hour}
}

func dp(h int) *metav1.Duration {
	d := d(h)
	return &d
}

func ref(name string) types.ObjectRef {
	return types.ObjectRef{Name: name, Namespace: "ns"}
}

func TestEvaluate_SingleOneShot(t *testing.T) {
	deadline := time.Now().Add(5 * 24 * time.Hour) // 5 days out
	thresholds := []Threshold{{Before: d(168)}}    // 7 days

	pending := Evaluate(deadline, thresholds, nil, time.Now())
	assert.Len(t, pending, 1)
	assert.Equal(t, d(168).Duration.String(), pending[0].Key)
}

func TestEvaluate_OneShotAlreadySent(t *testing.T) {
	now := time.Now()
	deadline := now.Add(5 * 24 * time.Hour)
	thresholds := []Threshold{{Before: d(168)}}
	sent := []SentReminder{{
		Threshold: d(168).Duration.String(),
		Ref:       ref("n1"),
		SentAt:    metav1.NewTime(now.Add(-1 * time.Hour)),
	}}

	pending := Evaluate(deadline, thresholds, sent, now)
	assert.Empty(t, pending)
}

func TestEvaluate_OutsideWindow(t *testing.T) {
	deadline := time.Now().Add(30 * 24 * time.Hour) // 30 days out
	thresholds := []Threshold{{Before: d(168)}}     // 7 day window

	pending := Evaluate(deadline, thresholds, nil, time.Now())
	assert.Empty(t, pending)
}

func TestEvaluate_PastDeadline_NeverSent_FiresLargestThreshold(t *testing.T) {
	now := time.Now()
	deadline := now.Add(-1 * time.Hour)
	thresholds := []Threshold{
		{Before: d(24)},
		{Before: d(168)},
	}

	pending := Evaluate(deadline, thresholds, nil, now)
	assert.Len(t, pending, 1)
	assert.Equal(t, d(168).Duration.String(), pending[0].Key)
}

func TestEvaluate_PastDeadline_AllSent_NothingFires(t *testing.T) {
	now := time.Now()
	deadline := now.Add(-1 * time.Hour)
	thresholds := []Threshold{
		{Before: d(24)},
		{Before: d(168)},
	}
	sent := []SentReminder{
		{Threshold: d(24).Duration.String(), Ref: ref("n1"), SentAt: metav1.NewTime(now.Add(-2 * time.Hour))},
		{Threshold: d(168).Duration.String(), Ref: ref("n2"), SentAt: metav1.NewTime(now.Add(-48 * time.Hour))},
	}

	pending := Evaluate(deadline, thresholds, sent, now)
	assert.Empty(t, pending)
}

func TestEvaluate_PastDeadline_PartiallySent_FiresLargestUnsent(t *testing.T) {
	now := time.Now()
	deadline := now.Add(-1 * time.Hour)
	thresholds := []Threshold{
		{Before: d(24)},
		{Before: d(168)},
	}
	// 168h was sent, but 24h was never sent
	sent := []SentReminder{
		{Threshold: d(168).Duration.String(), Ref: ref("n1"), SentAt: metav1.NewTime(now.Add(-48 * time.Hour))},
	}

	pending := Evaluate(deadline, thresholds, sent, now)
	assert.Len(t, pending, 1)
	assert.Equal(t, d(24).Duration.String(), pending[0].Key)
}

func TestEvaluate_MultipleThresholds_OnlyTightestFires(t *testing.T) {
	now := time.Now()
	deadline := now.Add(3 * 24 * time.Hour) // 3 days out

	thresholds := []Threshold{
		{Before: d(720)}, // 30 days — in window
		{Before: d(168)}, // 7 days — in window (tightest)
		{Before: d(24)},  // 1 day — NOT in window (3 days > 1 day)
	}

	// Nothing sent yet — only tightest (7d) should fire, not 30d
	pending := Evaluate(deadline, thresholds, nil, now)
	assert.Len(t, pending, 1)
	assert.Equal(t, d(168).Duration.String(), pending[0].Key)
}

func TestEvaluate_TightestOneShotSent_NothingFires(t *testing.T) {
	now := time.Now()
	deadline := now.Add(3 * 24 * time.Hour) // 3 days out

	thresholds := []Threshold{
		{Before: d(720)}, // 30 days — in window
		{Before: d(168)}, // 7 days — in window (tightest), already sent
		{Before: d(24)},  // 1 day — NOT in window
	}

	sent := []SentReminder{{
		Threshold: d(168).Duration.String(),
		Ref:       ref("n1"),
		SentAt:    metav1.NewTime(now.Add(-2 * 24 * time.Hour)),
	}}

	// Tightest is 7d, already sent (one-shot) → nothing fires
	// 30d is NOT evaluated because it's a larger threshold
	pending := Evaluate(deadline, thresholds, sent, now)
	assert.Empty(t, pending)
}

func TestEvaluate_RepeatNotYetDue(t *testing.T) {
	now := time.Now()
	deadline := now.Add(3 * 24 * time.Hour)
	thresholds := []Threshold{
		{Before: d(168), Repeat: dp(24)},
	}
	sent := []SentReminder{{
		Threshold: d(168).Duration.String(),
		Ref:       ref("n1"),
		SentAt:    metav1.NewTime(now.Add(-12 * time.Hour)), // 12h ago, repeat is 24h
	}}

	pending := Evaluate(deadline, thresholds, sent, now)
	assert.Empty(t, pending)
}

func TestEvaluate_RepeatDue(t *testing.T) {
	now := time.Now()
	deadline := now.Add(3 * 24 * time.Hour)
	thresholds := []Threshold{
		{Before: d(168), Repeat: dp(24)},
	}
	sent := []SentReminder{{
		Threshold: d(168).Duration.String(),
		Ref:       ref("n1"),
		SentAt:    metav1.NewTime(now.Add(-25 * time.Hour)), // 25h ago, repeat is 24h
	}}

	pending := Evaluate(deadline, thresholds, sent, now)
	assert.Len(t, pending, 1)
}

func TestNextRequeue_NotYetInWindow(t *testing.T) {
	now := time.Now()
	deadline := now.Add(10 * 24 * time.Hour)    // 10 days out
	thresholds := []Threshold{{Before: d(168)}} // 7 day window

	requeue := NextRequeue(deadline, thresholds, nil, now)
	// Should requeue in ~3 days (when we enter the 7-day window)
	expected := 10*24*time.Hour - 168*time.Hour
	assert.InDelta(t, expected.Seconds(), requeue.Seconds(), 1)
}

func TestNextRequeue_ShouldFireNow(t *testing.T) {
	now := time.Now()
	deadline := now.Add(5 * 24 * time.Hour)
	thresholds := []Threshold{{Before: d(168)}} // in window, never sent

	requeue := NextRequeue(deadline, thresholds, nil, now)
	assert.Equal(t, time.Duration(0), requeue)
}

func TestNextRequeue_RepeatScheduled(t *testing.T) {
	now := time.Now()
	deadline := now.Add(3 * 24 * time.Hour)
	thresholds := []Threshold{
		{Before: d(168), Repeat: dp(24)},
	}
	sent := []SentReminder{{
		Threshold: d(168).Duration.String(),
		Ref:       ref("n1"),
		SentAt:    metav1.NewTime(now.Add(-10 * time.Hour)), // 10h ago
	}}

	requeue := NextRequeue(deadline, thresholds, sent, now)
	// Next repeat in 24h - 10h = 14h
	assert.InDelta(t, 14*time.Hour.Seconds(), requeue.Seconds(), 1)
}

func TestNextRequeue_OneShotSent_WaitsForNextThreshold(t *testing.T) {
	now := time.Now()
	deadline := now.Add(3 * 24 * time.Hour) // 3 days out

	thresholds := []Threshold{
		{Before: d(168)}, // 7 days — in window, one-shot, sent
		{Before: d(24)},  // 1 day — not yet in window
	}
	sent := []SentReminder{{
		Threshold: d(168).Duration.String(),
		Ref:       ref("n1"),
		SentAt:    metav1.NewTime(now.Add(-1 * 24 * time.Hour)),
	}}

	requeue := NextRequeue(deadline, thresholds, sent, now)
	// Tightest active (168h) is sent. Next is 24h, which we enter in 3d - 1d = 2d.
	expected := 3*24*time.Hour - 24*time.Hour
	assert.InDelta(t, expected.Seconds(), requeue.Seconds(), 1)
}

func TestNextRequeue_PastDeadline(t *testing.T) {
	requeue := NextRequeue(time.Now().Add(-1*time.Hour), []Threshold{{Before: d(168)}}, nil, time.Now())
	assert.Equal(t, time.Duration(0), requeue)
}

func TestUpsertSent_Insert(t *testing.T) {
	entry := &SentReminder{Threshold: "168h0m0s", Ref: ref("n1"), SentAt: metav1.Now()}
	result := UpsertSent(nil, entry)
	assert.Len(t, result, 1)
	assert.Equal(t, "168h0m0s", result[0].Threshold)
}

func TestUpsertSent_Update(t *testing.T) {
	existing := []SentReminder{
		{Threshold: "168h0m0s", Ref: ref("n1"), SentAt: metav1.NewTime(time.Now().Add(-1 * time.Hour))},
	}
	updated := &SentReminder{Threshold: "168h0m0s", Ref: ref("n2"), SentAt: metav1.Now()}

	result := UpsertSent(existing, updated)
	assert.Len(t, result, 1)
	assert.Equal(t, "n2", result[0].Ref.Name)
}

func TestSortDesc(t *testing.T) {
	thresholds := []Threshold{
		{Before: d(24)},
		{Before: d(720)},
		{Before: d(168)},
	}
	sorted := SortDesc(thresholds)
	assert.Equal(t, d(720).Duration, sorted[0].Before.Duration)
	assert.Equal(t, d(168).Duration, sorted[1].Before.Duration)
	assert.Equal(t, d(24).Duration, sorted[2].Before.Duration)

	// Original unchanged
	assert.Equal(t, d(24).Duration, thresholds[0].Before.Duration)
}
