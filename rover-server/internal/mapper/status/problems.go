// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	roverStore "github.com/telekom/controlplane/rover-server/pkg/store"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

// SubResource constrains the types that can serve as sub-resources of a parent resource.
type SubResource interface {
	types.Object
	commonStore.Object
	*apiv1.Api | *apiv1.ApiSubscription | *apiv1.ApiExposure | *applicationv1.Application
}

// SubResourceChecker collects problems from one type of sub-resource owned by a parent resource.
type SubResourceChecker func(ctx context.Context, owner types.Object) ([]api.Problem, error)

// NewSubResourceChecker creates a SubResourceChecker that queries the given store
// for sub-resources owned by the parent and reports any that are not ready.
func NewSubResourceChecker[T SubResource](store commonStore.ObjectStore[T]) SubResourceChecker {
	return func(ctx context.Context, owner types.Object) ([]api.Problem, error) {
		return getAllProblemsInSubResource(ctx, owner, store)
	}
}

// GetAllRoverProblems retrieves all problems across all Rover sub-resource types.
//
// Each sub-resource type is queried via its store. If any query fails, the error
// is returned immediately and remaining sub-resource types are not checked.
func GetAllRoverProblems(ctx context.Context, rover *v1.Rover) ([]api.Problem, error) {
	checkers := []SubResourceChecker{
		NewSubResourceChecker(roverStore.ApiSubscriptionStore),
		NewSubResourceChecker(roverStore.ApiExposureStore),
		NewSubResourceChecker(roverStore.ApplicationStore),
	}

	var allProblems []api.Problem
	for _, check := range checkers {
		problems, err := check(ctx, rover)
		if err != nil {
			return nil, err
		}
		allProblems = append(allProblems, problems...)
	}
	return allProblems, nil
}

// GetAllRoverStateInfos retrieves all state information for a Rover including all sub-resources.
// It uses GetAllRoverProblems and maps problems to state information.
func GetAllRoverStateInfos(ctx context.Context, rover *v1.Rover) ([]api.StateInfo, error) {
	problems, err := GetAllRoverProblems(ctx, rover)
	if err != nil {
		return nil, err
	}
	return mapProblemsToStateInfos(problems), nil
}

// GetAllApiSpecificationProblems retrieves all problems across all ApiSpecification sub-resource types.
//
// Each sub-resource type is queried via its store. If any query fails, the error
// is returned immediately and remaining sub-resource types are not checked.
func GetAllApiSpecificationProblems(ctx context.Context, apiSpec *v1.ApiSpecification) ([]api.Problem, error) {
	checkers := []SubResourceChecker{
		NewSubResourceChecker(roverStore.ApiStore),
	}

	var allProblems []api.Problem
	for _, check := range checkers {
		problems, err := check(ctx, apiSpec)
		if err != nil {
			return nil, err
		}
		allProblems = append(allProblems, problems...)
	}
	return allProblems, nil
}

// GetAllApiSpecificationStateInfos retrieves all state information for an ApiSpecification including all sub-resources.
// It uses GetAllApiSpecificationProblems and maps problems to state information.
func GetAllApiSpecificationStateInfos(ctx context.Context, apiSpec *v1.ApiSpecification) ([]api.StateInfo, error) {
	problems, err := GetAllApiSpecificationProblems(ctx, apiSpec)
	if err != nil {
		return nil, err
	}
	return mapProblemsToStateInfos(problems), nil
}

// AnyRoverSubResourceStale returns true if any sub-resource of the given Rover
// has stale conditions (spec changed but controller hasn't reconciled yet).
func AnyRoverSubResourceStale(ctx context.Context, rover *v1.Rover) (bool, error) {
	stores := []stalenessChecker{
		newStalenessChecker(roverStore.ApiSubscriptionStore),
		newStalenessChecker(roverStore.ApiExposureStore),
		newStalenessChecker(roverStore.ApplicationStore),
	}
	for _, check := range stores {
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

// AnyApiSpecificationSubResourceStale returns true if any sub-resource of the
// given ApiSpecification has stale conditions.
func AnyApiSpecificationSubResourceStale(ctx context.Context, apiSpec *v1.ApiSpecification) (bool, error) {
	stores := []stalenessChecker{
		newStalenessChecker(roverStore.ApiStore),
	}
	for _, check := range stores {
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

// --- Internal helpers ---

// Lister abstracts the List method of an ObjectStore for testability.
type Lister[T types.Object] interface {
	List(ctx context.Context, opts commonStore.ListOpts) (*commonStore.ListResponse[T], error)
}

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
		if res == nil {
			continue
		}
		if isProcessingStale(res.GetConditions(), res.GetGeneration()) {
			return true, nil
		}
	}
	return false, nil
}

// getAllProblemsInSubResource queries the store for sub-resources owned by the given owner
// and returns a Problem for each sub-resource that has a not-ready condition.
func getAllProblemsInSubResource[T SubResource](ctx context.Context, owner types.Object, store Lister[T]) ([]api.Problem, error) {
	subResources, err := getAllSubResources(ctx, owner, store)
	if err != nil {
		return nil, err
	}

	problems := make([]api.Problem, 0)
	for _, res := range subResources {
		if res == nil {
			continue
		}
		if notReady := getNotReadyCondition(res.GetConditions()); notReady != nil {
			problems = append(problems, mapNotReadyConditionToProblem(notReady, res))
		}
	}
	return problems, nil
}

// getAllSubResources lists all sub-resources owned by owner via ownerReference UID filtering.
func getAllSubResources[T types.Object](ctx context.Context, owner types.Object, objStore Lister[T]) ([]T, error) {
	ownerUID := string(owner.GetUID())
	if ownerUID == "" {
		return nil, nil
	}

	listOpts := commonStore.NewListOpts()
	listOpts.Prefix = owner.GetNamespace()
	listOpts.Filters = []commonStore.Filter{
		{
			Path:  "metadata.ownerReferences.#.uid",
			Op:    commonStore.OpEqual,
			Value: ownerUID,
		},
	}

	resp, err := objStore.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// getNotReadyCondition returns the Ready condition if its status is False, or nil otherwise.
func getNotReadyCondition(conditions []metav1.Condition) *metav1.Condition {
	ready := meta.FindStatusCondition(conditions, condition.ConditionTypeReady)
	if ready != nil && ready.Status == metav1.ConditionFalse {
		return ready
	}
	return nil
}

// mapNotReadyConditionToProblem converts a not-ready condition and its owning object into an api.Problem.
func mapNotReadyConditionToProblem(cond *metav1.Condition, obj types.Object) api.Problem {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return api.Problem{
		Cause:   cond.Reason,
		Details: fmt.Sprintf("Condition: %s, Status: %s, Message: %s", cond.Type, cond.Status, cond.Message),
		Message: cond.Message,
		Resource: api.ResourceRef{
			ApiVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		},
	}
}

// mapProblemsToStateInfos converts a slice of Problems into a slice of StateInfos.
func mapProblemsToStateInfos(problems []api.Problem) []api.StateInfo {
	stateInfos := make([]api.StateInfo, 0, len(problems))
	for _, p := range problems {
		stateInfos = append(stateInfos, api.StateInfo{
			Message: p.Message,
			Cause:   p.Cause,
		})
	}
	return stateInfos
}
