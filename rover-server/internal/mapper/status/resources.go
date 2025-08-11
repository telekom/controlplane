// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

// SubResource defines an interface that all allowed types must implement
type SubResource interface {
	*apiv1.ApiSubscription | *apiv1.ApiExposure | *applicationv1.Application
	commonStore.Object
	GetConditions() []metav1.Condition
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

	for _, roverResource := range roverResources.Items {
		if roverResource != nil {
			ready := getNotReadyCondition(roverResource.GetConditions())

			if ready != nil {
				problem := mapNotReadyConditionToProblem(*ready, rover)
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
// - rover: The Rover resource associated with the condition.
//
// Returns:
// - api.Problem: The mapped problem.
func mapNotReadyConditionToProblem(condition metav1.Condition, rover *v1.Rover) api.Problem {
	contextInfo := "Application: " + rover.Status.Application.Name
	details := fmt.Sprintf("Condition: %s, Status: %s, Message: %s", condition.Type, condition.Status, condition.Message)
	return api.Problem{
		Cause:   condition.Reason,
		Details: details,
		Message: condition.Message,
		Context: contextInfo,
		Resource: api.ResourceRef{
			ApiVersion: rover.APIVersion,
			Kind:       rover.Kind,
			Name:       rover.Name,
			Namespace:  rover.Namespace,
		},
	}
}

// getAllSubResources retrieves all sub-resources for a given Rover resource from a store.
// It uses filters based on the Rover's namespace and owner reference.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose sub-resources are being retrieved.
// - store: The store containing the sub-resources.
//
// Returns:
// - *commonStore.ListResponse[T]: A pointer to a list response containing the sub-resources.
// - error: Any error encountered during the retrieval process.
func getAllSubResources[T SubResource](ctx context.Context, rover *v1.Rover, store commonStore.ObjectStore[T]) (*commonStore.ListResponse[T], error) {
	listOpts := commonStore.NewListOpts()

	if rover == nil || rover.Status.Application == nil {
		return nil, errors.New("rover resource is not processed and does not contain an application")
	}

	var namespaceFilter = []commonStore.Filter{
		{
			Path:  "metadata.namespace",
			Op:    commonStore.OpEqual,
			Value: rover.Status.Application.Namespace,
		},
	}

	var ownerFilter = []commonStore.Filter{
		{
			Path:  "metadata.ownerReferences.#.uid",
			Op:    commonStore.OpEqual,
			Value: string(rover.GetUID()),
		},
	}
	listOpts.Filters = append(namespaceFilter, ownerFilter...)

	return store.List(ctx, listOpts)
}
