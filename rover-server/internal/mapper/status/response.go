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
	v1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// MapResponse maps the status of a generic resource to a ResourceStatusResponse.
func MapResponse(ctx context.Context, obj types.Object) (api.ResourceStatusResponse, error) {
	status := MapStatus(obj.GetConditions(), obj.GetGeneration())

	processing := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
	var processedAtTime time.Time
	if processing != nil {
		processedAtTime = processing.LastTransitionTime.Time
	}

	return api.ResourceStatusResponse{
		CreatedAt:       obj.GetCreationTimestamp().Time,
		ProcessedAt:     processedAtTime,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
	}, nil
}

// MapApiSpecificationResponse maps the status of an ApiSpecification resource to a ResourceStatusResponse,
// including problems from sub-resources when the ApiSpecification itself is not complete.
// When the ApiSpecification is Complete/Done but any sub-resource has stale conditions,
// processingState is set to Processing to reflect that the overall pipeline is not yet done.
func MapApiSpecificationResponse(ctx context.Context, apiSpec *v1.ApiSpecification) (api.ResourceStatusResponse, error) {
	if apiSpec == nil {
		return api.ResourceStatusResponse{}, errors.New("input apiSpec is nil")
	}
	status := MapStatus(apiSpec.GetConditions(), apiSpec.GetGeneration())

	if status.State == api.Complete && status.ProcessingState == api.ProcessingStateDone {
		stale, err := AnyApiSpecificationSubResourceStale(ctx, apiSpec)
		if err != nil {
			return api.ResourceStatusResponse{}, err
		}
		if stale {
			status.ProcessingState = api.ProcessingStateProcessing
		}
	}

	var problems []api.Problem
	if status.State != api.Complete {
		var err error
		problems, err = GetAllApiSpecificationProblems(ctx, apiSpec)
		if err != nil {
			return api.ResourceStatusResponse{}, err
		}
	}

	processing := meta.FindStatusCondition(apiSpec.GetConditions(), condition.ConditionTypeProcessing)
	var processedAtTime time.Time
	if processing != nil {
		processedAtTime = processing.LastTransitionTime.Time
	}

	return api.ResourceStatusResponse{
		CreatedAt:       apiSpec.GetCreationTimestamp().Time,
		ProcessedAt:     processedAtTime,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
		Errors:          problems,
	}, nil
}

// MapRoverResponse maps the status of a Rover resource to a ResourceStatusResponse,
// including problems from sub-resources when the Rover itself is not complete.
// When the Rover is Complete/Done but any sub-resource has stale conditions,
// processingState is set to Processing to reflect that the overall pipeline is not yet done.
func MapRoverResponse(ctx context.Context, rover *v1.Rover) (api.ResourceStatusResponse, error) {
	if rover == nil {
		return api.ResourceStatusResponse{}, errors.New("input rover is nil")
	}
	status := MapStatus(rover.GetConditions(), rover.GetGeneration())

	if status.State == api.Complete && status.ProcessingState == api.ProcessingStateDone {
		stale, err := AnyRoverSubResourceStale(ctx, rover)
		if err != nil {
			return api.ResourceStatusResponse{}, err
		}
		if stale {
			status.ProcessingState = api.ProcessingStateProcessing
		}
	}

	var problems []api.Problem
	if status.State != api.Complete {
		var err error
		problems, err = GetAllRoverProblems(ctx, rover)
		if err != nil {
			return api.ResourceStatusResponse{}, err
		}
	}

	processing := meta.FindStatusCondition(rover.GetConditions(), condition.ConditionTypeProcessing)
	var processedAtTime time.Time
	if processing != nil {
		processedAtTime = processing.LastTransitionTime.Time
	}

	return api.ResourceStatusResponse{
		CreatedAt:       rover.GetCreationTimestamp().Time,
		ProcessedAt:     processedAtTime,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
		Errors:          problems,
	}, nil
}
