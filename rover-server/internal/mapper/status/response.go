// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// MapResponse maps the status of a generic resource to a ResourceStatusResponse.
func MapResponse(ctx context.Context, obj types.Object) (api.ResourceStatusResponse, error) {
	status := MapStatus(obj.GetConditions(), obj.GetGeneration())

	processing := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
	var processedAtTime time.Time
	if processing != nil {
		processedAtTime = processing.LastTransitionTime.Time.UTC()
	}

	return api.ResourceStatusResponse{
		CreatedAt:       obj.GetCreationTimestamp().Time.UTC(),
		ProcessedAt:     processedAtTime,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
	}, nil
}

// MapAPISpecificationResponse maps the status of an ApiSpecification resource to a ResourceStatusResponse,
// including problems from sub-resources when the ApiSpecification itself is not complete.
// When the ApiSpecification is Complete/Done but any sub-resource has stale conditions,
// processingState is set to Processing to reflect that the overall pipeline is not yet done.
func MapAPISpecificationResponse(ctx context.Context, apiSpec *v1.ApiSpecification, stores *store.Stores) (api.ResourceStatusResponse, error) {
	if apiSpec == nil {
		return api.ResourceStatusResponse{}, errors.New("input apiSpec is nil")
	}
	status := MapStatus(apiSpec.GetConditions(), apiSpec.GetGeneration())

	result, err := GetAllAPISpecificationProblems(ctx, apiSpec, stores)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	if status.State == api.Complete && status.ProcessingState == api.ProcessingStateDone && result.HasStale {
		status.ProcessingState = api.ProcessingStateProcessing
	}

	processing := meta.FindStatusCondition(apiSpec.GetConditions(), condition.ConditionTypeProcessing)
	var processedAtTime time.Time
	if processing != nil {
		processedAtTime = processing.LastTransitionTime.Time.UTC()
	}

	parentOverall := CalculateOverallStatus(status.State, status.ProcessingState)
	finalOverall := CompareAndReturn(parentOverall, result.WorstOverallStatus)

	return api.ResourceStatusResponse{
		CreatedAt:       apiSpec.GetCreationTimestamp().Time.UTC(),
		ProcessedAt:     processedAtTime,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   finalOverall,
		Errors:          result.Problems,
	}, nil
}

// MapRoverResponse maps the status of a Rover resource to a ResourceStatusResponse,
// including problems from sub-resources when the Rover itself is not complete.
// When the Rover is Complete/Done but any sub-resource has stale conditions,
// processingState is set to Processing to reflect that the overall pipeline is not yet done.
func MapRoverResponse(ctx context.Context, rover *v1.Rover, stores *store.Stores) (api.ResourceStatusResponse, error) {
	if rover == nil {
		return api.ResourceStatusResponse{}, errors.New("input rover is nil")
	}
	status := MapStatus(rover.GetConditions(), rover.GetGeneration())

	result, err := GetAllRoverProblems(ctx, rover, stores)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	if status.State == api.Complete && status.ProcessingState == api.ProcessingStateDone && result.HasStale {
		status.ProcessingState = api.ProcessingStateProcessing
	}

	processing := meta.FindStatusCondition(rover.GetConditions(), condition.ConditionTypeProcessing)
	var processedAtTime time.Time
	if processing != nil {
		processedAtTime = processing.LastTransitionTime.Time.UTC()
	}

	parentOverall := CalculateOverallStatus(status.State, status.ProcessingState)
	finalOverall := CompareAndReturn(parentOverall, result.WorstOverallStatus)

	return api.ResourceStatusResponse{
		CreatedAt:       rover.GetCreationTimestamp().Time.UTC(),
		ProcessedAt:     processedAtTime,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   finalOverall,
		Errors:          result.Problems,
	}, nil
}

// MapEventSpecificationResponse maps the status of an EventSpecification resource to a ResourceStatusResponse,
// including problems from sub-resources when the EventSpecification itself is not complete.
// When the EventSpecification is Complete/Done but any sub-resource has stale conditions,
// processingState is set to Processing to reflect that the overall pipeline is not yet done.
func MapEventSpecificationResponse(ctx context.Context, eventSpec *v1.EventSpecification, stores *store.Stores) (api.ResourceStatusResponse, error) {
	if eventSpec == nil {
		return api.ResourceStatusResponse{}, errors.New("input eventSpec is nil")
	}
	status := MapStatus(eventSpec.GetConditions(), eventSpec.GetGeneration())

	result, err := GetAllEventSpecificationProblems(ctx, eventSpec, stores)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	if status.State == api.Complete && status.ProcessingState == api.ProcessingStateDone && result.HasStale {
		status.ProcessingState = api.ProcessingStateProcessing
	}

	processing := meta.FindStatusCondition(eventSpec.GetConditions(), condition.ConditionTypeProcessing)
	var processedAtTime time.Time
	if processing != nil {
		processedAtTime = processing.LastTransitionTime.Time.UTC()
	}

	parentOverall := CalculateOverallStatus(status.State, status.ProcessingState)
	finalOverall := CompareAndReturn(parentOverall, result.WorstOverallStatus)

	return api.ResourceStatusResponse{
		CreatedAt:       eventSpec.GetCreationTimestamp().Time.UTC(),
		ProcessedAt:     processedAtTime,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   finalOverall,
		Errors:          result.Problems,
	}, nil
}
