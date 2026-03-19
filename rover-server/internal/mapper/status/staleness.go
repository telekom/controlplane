// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"

	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/types"
	roverStore "github.com/telekom/controlplane/rover-server/pkg/store"
	v1 "github.com/telekom/controlplane/rover/api/v1"
)

// stalenessChecker checks whether any sub-resource of an owner is stale.
type stalenessChecker func(ctx context.Context, owner types.Object) (bool, error)

// newStalenessChecker creates a stalenessChecker that queries the given store.
func newStalenessChecker[T SubResource](store commonStore.ObjectStore[T]) stalenessChecker {
	return func(ctx context.Context, owner types.Object) (bool, error) {
		return anySubResourceStale(ctx, owner, store)
	}
}

// anySubResourceStale lists sub-resources owned by owner and returns true
// if any has stale conditions (processing.ObservedGeneration < resource generation).
func anySubResourceStale[T SubResource](ctx context.Context, owner types.Object, store Lister[T]) (bool, error) {
	subResources, err := getAllSubResources(ctx, owner, store)
	if err != nil {
		return false, err
	}
	for _, res := range subResources {
		if isProcessingStale(res.GetConditions(), res.GetGeneration()) {
			return true, nil
		}
	}
	return false, nil
}

// AnyEventSpecificationSubResourceStale returns true if any sub-resource of the
// given EventSpecification has stale conditions.
// Store queries are skipped when the EventSpecification's status indicates zero
// sub-resources of the given type.
func AnyEventSpecificationSubResourceStale(ctx context.Context, eventSpec *v1.EventSpecification, stores *roverStore.Stores) (bool, error) {
	if eventSpec.Status.EventType.IsEmpty() {
		return false, nil
	}

	checkers := []stalenessChecker{
		newStalenessChecker(stores.EventTypeStore),
	}
	for _, check := range checkers {
		stale, err := check(ctx, eventSpec)
		if err != nil {
			return false, err
		}
		if stale {
			return true, nil
		}
	}
	return false, nil
}

// AnyAPISpecificationSubResourceStale returns true if any sub-resource of the
// given ApiSpecification has stale conditions.
// Store queries are skipped when the ApiSpecification's status indicates zero
// sub-resources of the given type.
func AnyAPISpecificationSubResourceStale(ctx context.Context, apiSpec *v1.ApiSpecification, stores *roverStore.Stores) (bool, error) {
	if apiSpec.Status.Api.IsEmpty() {
		return false, nil
	}

	checkers := []stalenessChecker{
		newStalenessChecker(stores.APIStore),
	}
	for _, check := range checkers {
		stale, err := check(ctx, apiSpec)
		if err != nil {
			return false, err
		}
		if stale {
			return true, nil
		}
	}
	return false, nil
}

// AnyRoverSubResourceStale returns true if any sub-resource of the given Rover
// has stale conditions (spec changed but controller hasn't reconciled yet).
// Store queries are skipped when the Rover's status indicates zero sub-resources
// of the given type.
func AnyRoverSubResourceStale(ctx context.Context, rover *v1.Rover, stores *roverStore.Stores) (bool, error) {
	var checkers []stalenessChecker
	if hasRefs(rover.Status.ApiSubscriptions) {
		checkers = append(checkers, newStalenessChecker(stores.APISubscriptionStore))
	}
	if hasRefs(rover.Status.ApiExposures) {
		checkers = append(checkers, newStalenessChecker(stores.APIExposureStore))
	}
	if rover.Status.Application != nil {
		checkers = append(checkers, newStalenessChecker(stores.ApplicationStore))
	}
	if hasRefs(rover.Status.EventExposures) {
		checkers = append(checkers, newStalenessChecker(stores.EventExposureStore))
	}
	if hasRefs(rover.Status.EventSubscriptions) {
		checkers = append(checkers, newStalenessChecker(stores.EventSubscriptionStore))
	}
	for _, check := range checkers {
		stale, err := check(ctx, rover)
		if err != nil {
			return false, err
		}
		if stale {
			return true, nil
		}
	}
	return false, nil
}
