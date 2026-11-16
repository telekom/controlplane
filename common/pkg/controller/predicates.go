// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = DeleteOnlyPredicate{}

// DeleteOnlyPredicate implements a predicate that only processes DELETE events.
// This is useful for watches where you only care about resource deletion,
// e.g. to trigger cleanup or re-reconciliation when a dependent resource is removed.
type DeleteOnlyPredicate struct {
	predicate.Funcs
}

func (DeleteOnlyPredicate) Create(e event.CreateEvent) bool {
	return false
}

func (DeleteOnlyPredicate) Delete(e event.DeleteEvent) bool {
	return true
}

func (DeleteOnlyPredicate) Update(e event.UpdateEvent) bool {
	return false
}

func (DeleteOnlyPredicate) Generic(e event.GenericEvent) bool {
	return false
}

var _ predicate.Predicate = SkipInitialListPredicate{}

// SkipInitialListPredicate drops the synthetic create events the informer emits
// for the objects already present when the cache is first filled. All other
// events, including creates observed after startup, pass.
//
// Use it on Watches sources whose map function only enqueues objects of the
// controller's For type: the For source already enqueues every primary object
// from its own initial list, so mapping the secondary initial list yields no new
// requests. Skipping it matters because controller-runtime waits for the initial
// list to pass through the predicates and event handler before the cache counts
// as synced; a map function that lists from the cache per event turns that into
// O(secondaries × primaries) work and can exceed CacheSyncTimeout.
//
// Pass it to Count as an inner predicate so the skipped events are recorded as
// ResultFiltered. Do not use it on For sources, or on Watches that enqueue
// objects not covered by the For initial list.
type SkipInitialListPredicate struct {
	predicate.Funcs
}

func (SkipInitialListPredicate) Create(e event.CreateEvent) bool { return !e.IsInInitialList }
