// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"

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
	types.Object       // implemented by any CR managed by the CP
	commonStore.Object // implemented by any CR stored in a ObjectStore
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
// Store queries are skipped when the Rover's status indicates zero sub-resources
// of the given type.
func GetAllRoverProblems(ctx context.Context, rover *v1.Rover, stores *roverStore.Stores) ([]api.Problem, error) {
	var checkers []SubResourceChecker
	if hasRefs(rover.Status.ApiSubscriptions) {
		checkers = append(checkers, NewSubResourceChecker(stores.APISubscriptionStore))
	}
	if hasRefs(rover.Status.ApiExposures) {
		checkers = append(checkers, NewSubResourceChecker(stores.APIExposureStore))
	}
	if rover.Status.Application != nil {
		checkers = append(checkers, NewSubResourceChecker(stores.ApplicationStore))
	}
	if hasRefs(rover.Status.EventExposures) {
		checkers = append(checkers, NewSubResourceChecker(stores.EventExposureStore))
	}
	if hasRefs(rover.Status.EventSubscriptions) {
		checkers = append(checkers, NewSubResourceChecker(stores.EventSubscriptionStore))
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
func GetAllRoverStateInfos(ctx context.Context, rover *v1.Rover, stores *roverStore.Stores) ([]api.StateInfo, error) {
	problems, err := GetAllRoverProblems(ctx, rover, stores)
	if err != nil {
		return nil, err
	}
	return mapProblemsToStateInfos(problems), nil
}

// GetAllAPISpecificationProblems retrieves all problems across all ApiSpecification sub-resource types.
//
// Each sub-resource type is queried via its store. If any query fails, the error
// is returned immediately and remaining sub-resource types are not checked.
// Store queries are skipped when the ApiSpecification's status indicates zero
// sub-resources of the given type.
func GetAllAPISpecificationProblems(ctx context.Context, apiSpec *v1.ApiSpecification, stores *roverStore.Stores) ([]api.Problem, error) {
	if apiSpec.Status.Api.IsEmpty() {
		return nil, nil
	}

	checkers := []SubResourceChecker{
		NewSubResourceChecker(stores.APIStore),
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

// GetAllAPISpecificationStateInfos retrieves all state information for an ApiSpecification including all sub-resources.
// It uses GetAllAPISpecificationProblems and maps problems to state information.
func GetAllAPISpecificationStateInfos(ctx context.Context, apiSpec *v1.ApiSpecification, stores *roverStore.Stores) ([]api.StateInfo, error) {
	problems, err := GetAllAPISpecificationProblems(ctx, apiSpec, stores)
	if err != nil {
		return nil, err
	}
	return mapProblemsToStateInfos(problems), nil
}

// GetAllEventSpecificationProblems retrieves all problems across all EventSpecification sub-resource types.
//
// Each sub-resource type is queried via its store. If any query fails, the error
// is returned immediately and remaining sub-resource types are not checked.
// Store queries are skipped when the EventSpecification's status indicates zero
// sub-resources of the given type.
func GetAllEventSpecificationProblems(ctx context.Context, eventSpec *v1.EventSpecification, stores *roverStore.Stores) ([]api.Problem, error) {
	if eventSpec.Status.EventType.IsEmpty() {
		return nil, nil
	}

	checkers := []SubResourceChecker{
		NewSubResourceChecker(stores.EventTypeStore),
	}

	var allProblems []api.Problem
	for _, check := range checkers {
		problems, err := check(ctx, eventSpec)
		if err != nil {
			return nil, err
		}
		allProblems = append(allProblems, problems...)
	}
	return allProblems, nil
}

// GetAllEventSpecificationStateInfos retrieves all state information for an EventSpecification including all sub-resources.
// It uses GetAllEventSpecificationProblems and maps problems to state information.
func GetAllEventSpecificationStateInfos(ctx context.Context, eventSpec *v1.EventSpecification, stores *roverStore.Stores) ([]api.StateInfo, error) {
	problems, err := GetAllEventSpecificationProblems(ctx, eventSpec, stores)
	if err != nil {
		return nil, err
	}
	return mapProblemsToStateInfos(problems), nil
}

// --- Internal helpers ---

// hasRefs returns true if the given slice of ObjectRefs is non-empty,
// indicating that sub-resources of this type exist according to the parent's status.
func hasRefs(refs []types.ObjectRef) bool {
	return len(refs) > 0
}

// Lister abstracts the List method of an ObjectStore for testability.
type Lister[T types.Object] interface {
	List(ctx context.Context, opts commonStore.ListOpts) (*commonStore.ListResponse[T], error)
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
