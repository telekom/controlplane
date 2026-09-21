// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"slices"

	"github.com/telekom/controlplane/common/pkg/condition"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// RouteRelevantForConsumeRoutePredicate admits a Route update to the
// ConsumeRoute controller only when a field the ConsumeRoute handler actually
// reads has changed: the Ready condition and Status.Consumers.
//
// A ResourceVersionChangedPredicate would admit every Route write instead. A
// Route reconciliation writes Status.Properties and its own conditions, and each
// of those writes then fans out to every ConsumeRoute of that Route, where the
// handler recomputes the same conditions and skips the status update. Route
// spec changes are irrelevant here for the same reason - the handler never looks
// at the spec.
//
// GenerationChangedPredicate cannot be used: the generation does not change on
// status writes, so it would drop exactly the events the ConsumeRoute needs to
// leave its blocked state.
type RouteRelevantForConsumeRoutePredicate struct{ predicate.Funcs }

func (RouteRelevantForConsumeRoutePredicate) Update(e event.UpdateEvent) bool {
	oldRoute, oldOk := e.ObjectOld.(*gatewayv1.Route)
	newRoute, newOk := e.ObjectNew.(*gatewayv1.Route)
	if !oldOk || !newOk {
		// Not a Route, so the assumptions above do not hold. Let the mapping
		// function decide rather than silently dropping the event.
		return true
	}

	wasReady := meta.IsStatusConditionTrue(oldRoute.GetConditions(), condition.ConditionTypeReady)
	isReady := meta.IsStatusConditionTrue(newRoute.GetConditions(), condition.ConditionTypeReady)
	if wasReady != isReady {
		return true
	}

	// Status.Consumers is sorted by the Route handler before it is stored, so a
	// plain comparison does not flap on ordering.
	return !slices.Equal(oldRoute.Status.Consumers, newRoute.Status.Consumers)
}
