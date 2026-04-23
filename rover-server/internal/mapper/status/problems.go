// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/rover-server/internal/api"
	roverStore "github.com/telekom/controlplane/rover-server/pkg/store"
	v1 "github.com/telekom/controlplane/rover/api/v1"
)

// SubResource constrains the types that can serve as sub-resources of a parent resource.
type SubResource interface {
	types.Object       // implemented by any CR managed by the CP
	commonStore.Object // implemented by any CR stored in a ObjectStore
}

// ProblemsResult holds problems found in sub-resources along with the worst
// OverallStatus observed across all sub-resources and whether any sub-resource
// has stale conditions (spec changed but controller hasn't reconciled yet).
type ProblemsResult struct {
	Problems           []api.Problem
	WorstOverallStatus api.OverallStatus
	HasStale           bool
}

// SubResourceChecker collects problems from one type of sub-resource owned by a parent resource
// and computes the worst OverallStatus across those sub-resources.
type SubResourceChecker func(ctx context.Context, owner types.Object) (ProblemsResult, error)

// NewSubResourceChecker creates a SubResourceChecker that queries the given store
// for sub-resources owned by the parent and reports any that are not ready.
// It also computes the worst OverallStatus across all sub-resources.
func NewSubResourceChecker[T SubResource](store commonStore.ObjectStore[T]) SubResourceChecker {
	return func(ctx context.Context, owner types.Object) (ProblemsResult, error) {
		return getAllProblemsInSubResource(ctx, owner, store)
	}
}

// GetAllRoverProblems retrieves all problems across all Rover sub-resource types
// and computes the worst OverallStatus observed across all sub-resources.
//
// Each sub-resource type is queried via its store. If any query fails, the error
// is returned immediately and remaining sub-resource types are not checked.
// Store queries are skipped when the Rover's status indicates zero sub-resources
// of the given type.
func GetAllRoverProblems(ctx context.Context, rover *v1.Rover, stores *roverStore.Stores) (ProblemsResult, error) {
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

	return runCheckers(ctx, rover, checkers)
}

// GetAllAPISpecificationProblems retrieves all problems across all ApiSpecification sub-resource types
// and computes the worst OverallStatus observed across all sub-resources.
//
// Each sub-resource type is queried via its store. If any query fails, the error
// is returned immediately and remaining sub-resource types are not checked.
// Store queries are skipped when the ApiSpecification's status indicates zero
// sub-resources of the given type.
func GetAllAPISpecificationProblems(ctx context.Context, apiSpec *v1.ApiSpecification, stores *roverStore.Stores) (ProblemsResult, error) {
	if apiSpec.Status.Api.IsEmpty() {
		return ProblemsResult{}, nil
	}

	checkers := []SubResourceChecker{
		NewSubResourceChecker(stores.APIStore),
	}

	return runCheckers(ctx, apiSpec, checkers)
}

// GetAllEventSpecificationProblems retrieves all problems across all EventSpecification sub-resource types
// and computes the worst OverallStatus observed across all sub-resources.
//
// Each sub-resource type is queried via its store. If any query fails, the error
// is returned immediately and remaining sub-resource types are not checked.
// Store queries are skipped when the EventSpecification's status indicates zero
// sub-resources of the given type.
func GetAllEventSpecificationProblems(ctx context.Context, eventSpec *v1.EventSpecification, stores *roverStore.Stores) (ProblemsResult, error) {
	if eventSpec.Status.EventType.IsEmpty() {
		return ProblemsResult{}, nil
	}

	checkers := []SubResourceChecker{
		NewSubResourceChecker(stores.EventTypeStore),
	}

	return runCheckers(ctx, eventSpec, checkers)
}

// --- Internal helpers ---

// runCheckers runs a list of SubResourceCheckers against the given owner and
// aggregates all problems and the worst OverallStatus across all sub-resource types.
func runCheckers(ctx context.Context, owner types.Object, checkers []SubResourceChecker) (ProblemsResult, error) {
	combined := ProblemsResult{}
	for _, check := range checkers {
		result, err := check(ctx, owner)
		if err != nil {
			return ProblemsResult{}, err
		}
		combined.Problems = append(combined.Problems, result.Problems...)
		combined.WorstOverallStatus = CompareAndReturn(combined.WorstOverallStatus, result.WorstOverallStatus)
		combined.HasStale = combined.HasStale || result.HasStale
	}
	return combined, nil
}

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
// and returns a ProblemsResult containing a Problem for each sub-resource that has a not-ready
// condition and the worst OverallStatus across all sub-resources.
func getAllProblemsInSubResource[T SubResource](ctx context.Context, owner types.Object, store Lister[T]) (ProblemsResult, error) {
	subResources, err := getAllSubResources(ctx, owner, store)
	if err != nil {
		return ProblemsResult{}, err
	}

	result := ProblemsResult{
		Problems: make([]api.Problem, 0),
	}
	for _, res := range subResources {
		subOverall := GetOverallStatus(res.GetConditions())
		result.WorstOverallStatus = CompareAndReturn(result.WorstOverallStatus, subOverall)

		if !result.HasStale && isProcessingStale(res.GetConditions(), res.GetGeneration()) {
			result.HasStale = true
		}

		if notReady := getNotReadyCondition(res.GetConditions()); notReady != nil {
			result.Problems = append(result.Problems, mapNotReadyConditionToProblem(notReady, res))
		}
	}
	return result, nil
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
