// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"

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
	}

	fillStateInfo(conditions, &status)
	return status
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

func GetOverallStatus(conditions []metav1.Condition) api.OverallStatus {
	status := MapStatus(conditions)
	return CalculateOverallStatus(status.State, status.ProcessingState)
}
