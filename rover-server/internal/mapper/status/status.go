// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	v1 "github.com/telekom/controlplane/rover/api/v1"
)

// Condition reason values. These must stay in sync with the factory functions
// in the condition package (e.g. condition.NewBlockedCondition, condition.NewDoneProcessingCondition).
const (
	reasonBlocked = "Blocked"
	reasonDone    = "Done"
)

// isProcessingStale returns true if the Processing condition's ObservedGeneration
// is behind the object's metadata.generation, indicating that the spec changed
// but the controller hasn't reconciled yet. Returns false when either generation
// is zero (backward compatibility or unknown generation).
func isProcessingStale(conditions []metav1.Condition, objectGeneration int64) bool {
	processing := meta.FindStatusCondition(conditions, condition.ConditionTypeProcessing)
	if processing == nil {
		return false
	}
	return objectGeneration > 0 && processing.ObservedGeneration > 0 && processing.ObservedGeneration < objectGeneration
}

// fillStateInfo maps Kubernetes Processing/Ready conditions into State, ProcessingState,
// and any associated warnings or errors on the given status.
// objectGeneration is the resource's metadata.generation; when a condition's
// ObservedGeneration is non-zero but less than objectGeneration, the condition
// is stale (spec changed but controller hasn't reconciled yet) and the status
// is set to pending. Pass 0 to skip staleness detection.
func fillStateInfo(conditions []metav1.Condition, objectGeneration int64, status *api.Status) {
	processing := meta.FindStatusCondition(conditions, condition.ConditionTypeProcessing)
	if processing == nil {
		status.State = api.None
		status.ProcessingState = api.ProcessingStateNone
		status.Warnings = []api.StateInfo{
			{Message: "Processing condition not found"},
		}
		return
	}

	// Staleness detection: if the controller has started reporting ObservedGeneration
	// (> 0) but hasn't caught up to the current spec generation, the condition
	// values are based on an older spec and cannot be trusted.
	if isProcessingStale(conditions, objectGeneration) {
		status.State = api.None
		status.ProcessingState = api.ProcessingStatePending
		return
	}

	ready := meta.FindStatusCondition(conditions, condition.ConditionTypeReady)
	if ready == nil {
		status.State = api.None
		status.ProcessingState = api.ProcessingStateNone
		status.Warnings = []api.StateInfo{
			{Message: "Ready condition not found"},
		}
		return
	}

	if processing.Status == metav1.ConditionTrue {
		status.State = api.Blocked
		status.ProcessingState = api.ProcessingStateProcessing
		return
	}

	if processing.Reason == reasonBlocked {
		status.State = api.Blocked
		status.ProcessingState = api.ProcessingStateDone
		status.Warnings = []api.StateInfo{
			{Message: processing.Message},
		}
		return
	}

	if processing.Reason == reasonDone {
		status.ProcessingState = api.ProcessingStateDone
		if ready.Status == metav1.ConditionTrue {
			status.State = api.Complete
		} else {
			status.State = api.Blocked
			status.Warnings = []api.StateInfo{
				{Message: ready.Message},
			}
		}
		return
	}

	// Fallthrough: processing failed (reason is neither Blocked nor Done).
	status.State = api.Invalid
	status.ProcessingState = api.ProcessingStateFailed
	status.Errors = []api.StateInfo{
		{Message: processing.Message},
	}
}

// MapStatus maps a set of Kubernetes conditions to an api.Status.
// objectGeneration is the resource's metadata.generation used for staleness
// detection. Pass 0 to skip staleness detection (e.g. when only conditions
// are available without the parent object).
func MapStatus(conditions []metav1.Condition, objectGeneration int64) api.Status {
	status := api.Status{
		ProcessingState: api.ProcessingStateNone,
		State:           api.None,
	}
	fillStateInfo(conditions, objectGeneration, &status)
	return status
}

// MapRoverStatus maps the status of a Rover resource to an api.Status,
// including sub-resource error information when the Rover itself is not complete.
// When the Rover's own conditions indicate Complete/Done but any sub-resource
// has stale conditions, processingState is set to Processing to reflect that
// the overall pipeline is not yet done.
func MapRoverStatus(ctx context.Context, rover *v1.Rover, stores *store.Stores) (api.Status, error) {
	status := MapStatus(rover.GetConditions(), rover.GetGeneration())

	result, err := GetAllRoverProblems(ctx, rover, stores)
	if err != nil {
		return status, err
	}

	if status.State == api.Complete && status.ProcessingState == api.ProcessingStateDone && result.HasStale {
		status.ProcessingState = api.ProcessingStateProcessing
	}

	status.Errors = append(status.Errors, mapProblemsToStateInfos(result.Problems)...)

	return status, nil
}

// MapAPISpecificationStatus maps the status of an ApiSpecification resource to an api.Status,
// including sub-resource error information when the ApiSpecification itself is not complete.
// When the ApiSpecification's own conditions indicate Complete/Done but any sub-resource
// has stale conditions, processingState is set to Processing to reflect that
// the overall pipeline is not yet done.
func MapAPISpecificationStatus(ctx context.Context, apiSpec *v1.ApiSpecification, stores *store.Stores) (api.Status, error) {
	status := MapStatus(apiSpec.GetConditions(), apiSpec.GetGeneration())

	result, err := GetAllAPISpecificationProblems(ctx, apiSpec, stores)
	if err != nil {
		return status, err
	}

	if status.State == api.Complete && status.ProcessingState == api.ProcessingStateDone && result.HasStale {
		status.ProcessingState = api.ProcessingStateProcessing
	}

	status.Errors = append(status.Errors, mapProblemsToStateInfos(result.Problems)...)

	return status, nil
}

// MapEventSpecificationStatus maps the status of an EventSpecification resource to an api.Status,
// including sub-resource error information when the EventSpecification itself is not complete.
// When the EventSpecification's own conditions indicate Complete/Done but any sub-resource
// has stale conditions, processingState is set to Processing to reflect that
// the overall pipeline is not yet done.
func MapEventSpecificationStatus(ctx context.Context, eventSpec *v1.EventSpecification, stores *store.Stores) (api.Status, error) {
	status := MapStatus(eventSpec.GetConditions(), eventSpec.GetGeneration())

	result, err := GetAllEventSpecificationProblems(ctx, eventSpec, stores)
	if err != nil {
		return status, err
	}

	if status.State == api.Complete && status.ProcessingState == api.ProcessingStateDone && result.HasStale {
		status.ProcessingState = api.ProcessingStateProcessing
	}

	status.Errors = append(status.Errors, mapProblemsToStateInfos(result.Problems)...)

	return status, nil
}

// GetOverallStatus computes the OverallStatus from a set of Kubernetes conditions.
// Note: staleness detection is not performed here because the object's generation
// is not available. Callers that need staleness detection should use MapStatus directly.
func GetOverallStatus(conditions []metav1.Condition) api.OverallStatus {
	status := MapStatus(conditions, 0)
	return CalculateOverallStatus(status.State, status.ProcessingState)
}

// CalculateOverallStatus collapses a State and ProcessingState into a single OverallStatus.
func CalculateOverallStatus(s api.State, ps api.ProcessingState) api.OverallStatus {
	if ps == api.ProcessingStateProcessing {
		return api.OverallStatusProcessing
	}
	if ps == api.ProcessingStateFailed {
		return api.OverallStatusFailed
	}
	if s == api.Blocked {
		return api.OverallStatusBlocked
	}
	if ps == api.ProcessingStatePending {
		return api.OverallStatusPending
	}
	if s == api.Complete && ps == api.ProcessingStateDone {
		return api.OverallStatusComplete
	}
	return api.OverallStatusNone
}

// statusPriority defines severity ordering for OverallStatus values.
// Higher values indicate more severe statuses.
// Unknown or unmapped statuses get priority 0 (least severe), so they never
// silently shadow a known status in CompareAndReturn.
var statusPriority = map[api.OverallStatus]int{
	api.OverallStatusInvalid:    7,
	api.OverallStatusFailed:     6,
	api.OverallStatusBlocked:    5,
	api.OverallStatusProcessing: 4,
	api.OverallStatusPending:    3,
	api.OverallStatusNone:       2,
	api.OverallStatusComplete:   1,
	api.OverallStatusDone:       1,
}

// CompareAndReturn returns the more severe of two OverallStatus values.
// Priority (highest to lowest): Invalid > Failed > Blocked > Processing > Pending > None > Complete = Done.
// Unknown statuses are treated as least severe (priority 0).
func CompareAndReturn(a, b api.OverallStatus) api.OverallStatus {
	if statusPriority[a] >= statusPriority[b] {
		return a
	}
	return b
}
