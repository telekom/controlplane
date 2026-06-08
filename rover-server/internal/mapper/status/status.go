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
	"github.com/telekom/controlplane/rover-server/pkg/store"
)

// processingReasons lists Ready condition reasons that indicate the resource
// is actively being processed (transient, will resolve on its own).
// All other Ready=False reasons are treated as blocked/failed.
var processingReasons = map[string]bool{
	condition.ReasonProcessing:          true, // "Processing"
	condition.ReasonSubResourceNotReady: true, // "SubResourceNotReady"
	"Provisioning":                      true,
}

// isStale returns true if a condition's ObservedGeneration is behind the
// object's metadata.generation, indicating that the spec changed but the
// controller hasn't reconciled yet. Returns false when either generation
// is zero (backward compatibility or unknown generation) or when the
// condition is nil.
func isStale(cond *metav1.Condition, objectGeneration int64) bool {
	if cond == nil {
		return false
	}
	return objectGeneration > 0 && cond.ObservedGeneration > 0 && cond.ObservedGeneration < objectGeneration
}

// isProcessingStale is a convenience wrapper that checks staleness on the
// Processing condition. Retained for use outside fillStateInfo.
func isProcessingStale(conditions []metav1.Condition, objectGeneration int64) bool {
	processing := meta.FindStatusCondition(conditions, condition.ConditionTypeProcessing)
	return isStale(processing, objectGeneration)
}

// fillStateInfo derives State and ProcessingState from Kubernetes conditions.
//
// The Ready condition is the primary driver:
//   - Ready=True → Complete (resource is functional)
//   - Ready=False with a processing-equivalent reason → Processing (actively progressing)
//   - Ready=False with any other reason → Blocked (cannot progress)
//
// The Processing condition is used as a supplementary signal:
//   - Fallback when Ready is not informative (Unknown or contradictory)
//   - Processing.Status=True confirms active work regardless of Ready reason
//
// objectGeneration is the resource's metadata.generation. Pass 0 to skip
// staleness detection.
func fillStateInfo(conditions []metav1.Condition, objectGeneration int64, status *api.Status) {
	processing := meta.FindStatusCondition(conditions, condition.ConditionTypeProcessing)
	ready := meta.FindStatusCondition(conditions, condition.ConditionTypeReady)

	// --- No conditions at all ---
	if processing == nil && ready == nil {
		status.State = api.None
		status.ProcessingState = api.ProcessingStateNone
		status.Warnings = []api.StateInfo{
			{Message: "No conditions found"},
		}
		return
	}

	// --- Staleness detection  ---
	if isStale(ready, objectGeneration) {
		status.State = api.None
		status.ProcessingState = api.ProcessingStatePending
		return
	}

	// --- Ready condition is the primary driver ---
	if ready != nil && ready.Status == metav1.ConditionTrue {
		// Ready=True: resource is functional.
		// Guard against stale Ready when Processing explicitly says Blocked.
		if processing != nil && processing.Reason == "Blocked" {
			// Contradictory: Ready=True but Processing=Blocked → trust Processing (Ready is stale).
			status.State = api.Blocked
			status.ProcessingState = api.ProcessingStateDone
			status.Warnings = []api.StateInfo{
				{Message: processing.Message},
			}
			return
		}
		status.State = api.Complete
		status.ProcessingState = api.ProcessingStateDone
		return
	}

	if ready != nil && ready.Status == metav1.ConditionFalse {
		// Ready=False: check if processing-equivalent reason or blocked.
		if processingReasons[ready.Reason] {
			status.State = api.None
			status.ProcessingState = api.ProcessingStateProcessing
			return
		}
		// Not a processing reason → resource is blocked.
		status.State = api.Blocked
		status.ProcessingState = api.ProcessingStateDone
		status.Warnings = []api.StateInfo{
			{Message: ready.Message},
		}
		return
	}

	// --- Ready is Unknown or nil: fall back to Processing condition ---
	if processing == nil {
		status.State = api.None
		status.ProcessingState = api.ProcessingStateNone
		status.Warnings = []api.StateInfo{
			{Message: "Ready condition not found"},
		}
		return
	}

	if processing.Status == metav1.ConditionTrue {
		status.State = api.None
		status.ProcessingState = api.ProcessingStateProcessing
		return
	}

	if processing.Reason == "Blocked" {
		status.State = api.Blocked
		status.ProcessingState = api.ProcessingStateDone
		status.Warnings = []api.StateInfo{
			{Message: processing.Message},
		}
		return
	}

	if processing.Reason == "Done" {
		status.State = api.None
		status.ProcessingState = api.ProcessingStateDone
		return
	}

	// Fallthrough: unknown state.
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

// GetProcessingState computes the ProcessingState from a set of Kubernetes conditions.
// Note: staleness detection is not performed here because the object's generation
// is not available. Callers that need staleness detection should use MapStatus directly.
func GetProcessingState(conditions []metav1.Condition) api.ProcessingState {
	status := MapStatus(conditions, 0)
	return status.ProcessingState
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
