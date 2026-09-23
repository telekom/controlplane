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
// for the objects already present when the cache is first filled. Those objects
// are reconciled by the periodic requeue anyway, so admitting them only makes
// every restart pay a full resync burst.
type SkipInitialListPredicate struct {
	predicate.Funcs
}

func (SkipInitialListPredicate) Create(e event.CreateEvent) bool { return !e.IsInInitialList }
