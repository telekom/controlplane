// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"time"

	ghErrors "github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func fillStateInfo(conditions []metav1.Condition, status *api.Status) {
	processing := meta.FindStatusCondition(conditions, condition.ConditionTypeProcessing)
	if processing == nil {
		status.State = api.None
		status.ProcessingState = api.ProcessingStateNone
		status.Warnings = []api.StateInfo{
			{
				Message: "Processing condition not found",
			},
		}
		return
	}

	ready := meta.FindStatusCondition(conditions, condition.ConditionTypeReady)
	if ready == nil {
		status.State = api.None
		status.ProcessingState = api.ProcessingStateNone
		status.Warnings = []api.StateInfo{
			{
				Message: "Ready condition not found",
			},
		}
		return
	}

	if processing.Status == metav1.ConditionTrue {
		status.State = api.Blocked
		status.ProcessingState = api.ProcessingStateProcessing
		return
	}

	if processing.Reason == "Blocked" {
		status.State = api.Blocked
		status.ProcessingState = api.ProcessingStateDone
		status.Warnings = []api.StateInfo{
			{
				Message: processing.Message,
			},
		}
		return
	}

	if processing.Reason == "Done" {
		status.ProcessingState = api.ProcessingStateDone
		if ready.Status == metav1.ConditionTrue {
			status.State = api.Complete
		} else {
			status.State = api.Blocked
			status.Warnings = []api.StateInfo{
				{
					Message: ready.Message,
				},
			}
		}

		return
	}

	status.ProcessingState = api.ProcessingStateFailed
	status.Errors = []api.StateInfo{
		{
			Message: processing.Message,
		},
	}
}

func MapStatus(conditions []metav1.Condition) api.Status {
	status := api.Status{
		ProcessingState: api.ProcessingStateNone,
		State:           api.None,
		Time:            time.Now().Format(time.RFC3339),
	}

	fillStateInfo(conditions, &status)
	return status
}

func MapResponse(conditions []metav1.Condition) (api.ResourceStatusResponse, error) {
	status := MapStatus(conditions)

	return api.ResourceStatusResponse{
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
	}, nil
}

// MapRoverStatus maps the status of a Rover resource to a Rover API status.
// It retrieves the conditions of the Rover, maps them to a Rover API status,
// and checks for any sub-resource conditions with error states.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose status is being mapped.
//
// Returns:
// - *api.Status: The mapped status of the Rover resource.
func MapRoverStatus(ctx context.Context, rover *v1.Rover) api.Status {
	status := MapStatus(rover.GetConditions())
	var stateInfos = []api.StateInfo{}

	if status.State != api.Complete {
		// Load all sub resources and check for conditions with error state
		stateInfos = AppendStateInfos(stateInfos, GetAllStateInfos(ctx, rover))
		status.Errors = stateInfos
	}

	return status
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

	return api.ResourceStatusResponse{
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
		Errors:          errors,
	}, nil
}

func GetOverallStatus(conditions []metav1.Condition) api.OverallStatus {
	status := MapStatus(conditions)
	return CalculateOverallStatus(status.State, status.ProcessingState)
}
