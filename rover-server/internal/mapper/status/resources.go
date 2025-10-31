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
	cstore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

// SubResource defines an interface that all allowed types must implement
type SubResource interface {
	types.Object
	commonStore.Object
	*apiv1.ApiSubscription | *apiv1.ApiExposure | *applicationv1.Application
}

// GetAllProblemsInSubResource retrieves all problems in a sub-resource for a given Rover resource.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose sub-resource problems are being retrieved.
// - store: The store containing the sub-resource.
//
// Returns:
// - []api.Problem: A slice of problems.
// - error: Any error encountered during the retrieval process.
func GetAllProblemsInSubResource[T SubResource](ctx context.Context, rover *v1.Rover, store commonStore.ObjectStore[T]) ([]api.Problem, error) {
	var errors = []api.Problem{}
	roverResources, err := getAllSubResources(ctx, rover, store)
	if err != nil {
		return nil, err
	}

	for _, roverResource := range roverResources {
		if roverResource != nil {
			ready := getNotReadyCondition(roverResource.GetConditions())

			if ready != nil {
				problem := mapNotReadyConditionToProblem(*ready, roverResource)
				errors = append(errors, problem)
			}
		}
	}
	return errors, nil
}

// getNotReadyCondition retrieves the "not ready" condition from a slice of conditions.
//
// Parameters:
// - conditions: A slice of conditions.
//
// Returns:
// - *metav1.Condition: A pointer to the "not ready" condition, if found.
func getNotReadyCondition(conditions []metav1.Condition) *metav1.Condition {
	ready := meta.FindStatusCondition(conditions, condition.ConditionTypeReady)
	if ready != nil && ready.Status == metav1.ConditionFalse {
		return ready
	} else {
		return nil
	}
}

// mapNotReadyConditionToProblem maps a "not ready" condition to a problem.
//
// Parameters:
// - condition: The "not ready" condition to be mapped.
// - obj: The object associated with the condition.
//
// Returns:
// - api.Problem: The mapped problem.
func mapNotReadyConditionToProblem(condition metav1.Condition, obj types.Object) api.Problem {
	details := fmt.Sprintf("Condition: %s, Status: %s, Message: %s", condition.Type, condition.Status, condition.Message)
	gvk := obj.GetObjectKind().GroupVersionKind()
	return api.Problem{
		Cause:   condition.Reason,
		Details: details,
		Message: condition.Message,
		Resource: api.ResourceRef{
			ApiVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		},
	}
}

type Lister[T types.Object] interface {
	List(ctx context.Context, opts cstore.ListOpts) (*cstore.ListResponse[T], error)
}

func getAllSubResources[T types.Object](ctx context.Context, owner types.Object, objStore Lister[T]) ([]T, error) {
	ownerUid := string(owner.GetUID())
	if ownerUid == "" {
		return nil, nil
	}
	filters := []cstore.Filter{
		{
			Path:  "metadata.ownerReferences.#.uid",
			Op:    cstore.OpEqual,
			Value: ownerUid,
		},
	}

	listOpts := cstore.NewListOpts()
	listOpts.Prefix = owner.GetNamespace()
	listOpts.Filters = filters

	subObjects, err := objStore.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}
	return subObjects.Items, nil
}
