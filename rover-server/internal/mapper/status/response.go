// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"

	ghErrors "github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/rover-server/internal/api"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// MapResponse maps the status of a generic resource to a ResourceStatusResponse.
func MapResponse(ctx context.Context, obj types.Object) (api.ResourceStatusResponse, error) {
	status := MapStatus(obj.GetConditions())
	processing := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)

	return api.ResourceStatusResponse{
		CreatedAt:       obj.GetCreationTimestamp().Time,
		ProcessedAt:     processing.LastTransitionTime.Time,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
	}, nil
}

// MapApiSpecificationResponse maps the status of an ApiSpecification resource to a ResourceStatusResponse.
func MapApiSpecificationResponse(ctx context.Context, apiSpec *v1.ApiSpecification) (api.ResourceStatusResponse, error) {
	if apiSpec == nil {
		return api.ResourceStatusResponse{}, ghErrors.New("input apiSpec is nil")
	}
	status := MapStatus(apiSpec.GetConditions())
	processing := meta.FindStatusCondition(apiSpec.GetConditions(), condition.ConditionTypeProcessing)

	return api.ResourceStatusResponse{
		CreatedAt:       apiSpec.GetCreationTimestamp().Time,
		ProcessedAt:     processing.LastTransitionTime.Time,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
	}, nil
}

// MapRoverResponse maps the status of a Rover resource to a ResourceStatusResponse.
// It retrieves the conditions of the Rover, maps them to a status, and checks for any sub-resource
// conditions with error states.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose status is being mapped.
//
// Returns:
// - api.ResourceStatusResponse: The mapped status response of the Rover resource.
// - error: Any error encountered during the mapping process.
func MapRoverResponse(ctx context.Context, rover *v1.Rover) (api.ResourceStatusResponse, error) {
	if rover == nil {
		return api.ResourceStatusResponse{}, ghErrors.New("input rover is nil")
	}
	status := MapStatus(rover.GetConditions())
	var errors = []api.Problem{}

	if status.State != api.Complete {
		// Load all sub resources and check for conditions with error state
		errors = append(errors, GetAllProblems(ctx, rover)...)
	}

	processing := meta.FindStatusCondition(rover.GetConditions(), condition.ConditionTypeProcessing)

	return api.ResourceStatusResponse{
		CreatedAt:       rover.GetCreationTimestamp().Time,
		ProcessedAt:     processing.LastTransitionTime.Time,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
		Errors:          errors,
	}, nil
}
