// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package reminder

import (
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PendingReminder represents a threshold that should fire now.
type PendingReminder struct {
	// Threshold is the configuration entry that triggered this reminder.
	Threshold Threshold
	// Key is the string representation of Threshold.Before (e.g. "720h0m0s"),
	// used for deduplication and tracking.
	Key string
}

// Evaluate determines which threshold should fire given the current time,
// a deadline, the configured thresholds, and which reminders have already been sent.
//
// Only the tightest (smallest) matching threshold is evaluated to avoid spamming
// the user with multiple notifications at once. For one-shot thresholds (no Repeat),
// a reminder fires once. For repeating thresholds, a reminder fires again once the
// Repeat interval has elapsed since the last send.
//
// Returns at most one PendingReminder per call.
func Evaluate(deadline time.Time, thresholds []Threshold, sent []SentReminder, now time.Time) []PendingReminder {
	timeUntilDeadline := deadline.Sub(now)
	sentIndex := indexSent(sent)

	// If the deadline has already passed, fire the widest unsent threshold
	// so the user is informed even if the reconciler woke up too late
	// (e.g. due to jitter or controller downtime).
	if timeUntilDeadline <= 0 {
		descSorted := SortDesc(thresholds)
		for _, t := range descSorted {
			key := t.Before.Duration.String()
			if _, ok := sentIndex[key]; !ok {
				return []PendingReminder{{
					Threshold: t,
					Key:       key,
				}}
			}
		}
		return nil // all thresholds already sent
	}

	sorted := sortAsc(thresholds)

	// Find the tightest threshold whose window we are in.
	for _, t := range sorted {
		if timeUntilDeadline > t.Before.Duration {
			continue // not yet in this threshold's window
		}

		// This is the tightest matching threshold.
		key := t.Before.Duration.String()
		if alreadySent(sentIndex, key, t.Repeat, now) {
			return nil
		}

		return []PendingReminder{{
			Threshold: t,
			Key:       key,
		}}
	}

	return nil
}

// NextRequeue computes the duration until the next reminder event, which can be
// used as a RequeueAfter value. It returns 0 if there is nothing to schedule
// (e.g. the deadline has passed or a reminder should fire immediately).
func NextRequeue(deadline time.Time, thresholds []Threshold, sent []SentReminder, now time.Time) time.Duration {
	timeUntilDeadline := deadline.Sub(now)
	if timeUntilDeadline <= 0 {
		return 0
	}

	sorted := sortAsc(thresholds)
	sentIndex := indexSent(sent)

	// Find the tightest threshold whose window we are in.
	for _, t := range sorted {
		if timeUntilDeadline > t.Before.Duration {
			continue // not yet in this window
		}

		// This is the tightest matching threshold.
		key := t.Before.Duration.String()

		if t.Repeat != nil {
			if s, ok := sentIndex[key]; ok {
				nextFire := s.SentAt.Time.Add(t.Repeat.Duration).Sub(now)
				if nextFire > 0 {
					return nextFire
				}
				return 0 // overdue, fire now
			}
			return 0 // never sent, fire now
		}

		// One-shot
		if _, ok := sentIndex[key]; ok {
			// Already sent. Look for the next smaller threshold to enter.
			// Fall through to check if there's a tighter threshold below.
			continue
		}
		return 0 // never sent, fire now
	}

	// No threshold is active. Find when we'll enter the next window.
	// Thresholds are sorted ascending, so the last one with Before > timeUntilDeadline
	// hasn't been entered yet. We want the largest Before that we haven't entered.
	descSorted := SortDesc(thresholds)
	for _, t := range descSorted {
		if timeUntilDeadline > t.Before.Duration {
			return timeUntilDeadline - t.Before.Duration
		}
	}

	return timeUntilDeadline
}

// UpsertSent inserts or updates a SentReminder in the slice, matching by Threshold key.
// If a matching entry exists, its Ref and SentAt are updated. Otherwise a new entry is appended.
func UpsertSent(sent []SentReminder, entry *SentReminder) []SentReminder {
	for i, s := range sent {
		if s.Threshold == entry.Threshold {
			sent[i].Ref = entry.Ref
			sent[i].SentAt = entry.SentAt
			return sent
		}
	}
	return append(sent, *entry)
}

// SortDesc returns a copy of the thresholds sorted descending by Before duration (largest first).
func SortDesc(thresholds []Threshold) []Threshold {
	sorted := make([]Threshold, len(thresholds))
	copy(sorted, thresholds)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Before.Duration > sorted[j].Before.Duration
	})
	return sorted
}

// sortAsc returns a copy of the thresholds sorted ascending by Before duration (smallest first).
func sortAsc(thresholds []Threshold) []Threshold {
	sorted := make([]Threshold, len(thresholds))
	copy(sorted, thresholds)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Before.Duration < sorted[j].Before.Duration
	})
	return sorted
}

// alreadySent checks if a reminder for the given threshold key has already been
// sent and should not fire again.
func alreadySent(sentIndex map[string]SentReminder, key string, repeat *metav1.Duration, now time.Time) bool {
	s, ok := sentIndex[key]
	if !ok {
		return false
	}
	if repeat == nil {
		return true // one-shot, already sent
	}
	// Repeating: check if enough time has passed since last send
	return now.Sub(s.SentAt.Time) < repeat.Duration
}

// indexSent builds a lookup map from threshold key to SentReminder.
func indexSent(sent []SentReminder) map[string]SentReminder {
	m := make(map[string]SentReminder, len(sent))
	for _, s := range sent {
		m[s.Threshold] = s
	}
	return m
}
