// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"github.com/telekom/controlplane/common/pkg/condition"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/spy-server/internal/api"
)

const (
	reasonBlocked = "Blocked"
	reasonDone    = "Done"
)

// isProcessingStale returns true if the Processing condition's ObservedGeneration
// is behind the object's metadata.generation.
func isProcessingStale(conditions []metav1.Condition, objectGeneration int64) bool {
	processing := meta.FindStatusCondition(conditions, condition.ConditionTypeProcessing)
	if processing == nil {
		return false
	}
	return objectGeneration > 0 && processing.ObservedGeneration > 0 && processing.ObservedGeneration < objectGeneration
}

// fillStateInfo maps Kubernetes Processing/Ready conditions into State, ProcessingState,
// and any associated warnings or errors on the given status.
func fillStateInfo(conditions []metav1.Condition, objectGeneration int64, status *api.Status) {
	processing := meta.FindStatusCondition(conditions, condition.ConditionTypeProcessing)
	if processing == nil {
		status.State = api.StateNone
		status.ProcessingState = api.ProcessingStateNone
		status.Warnings = []api.StateInfo{
			{Message: "Processing condition not found"},
		}
		return
	}

	if isProcessingStale(conditions, objectGeneration) {
		status.State = api.StateNone
		status.ProcessingState = api.ProcessingStatePending
		return
	}

	ready := meta.FindStatusCondition(conditions, condition.ConditionTypeReady)
	if ready == nil {
		status.State = api.StateNone
		status.ProcessingState = api.ProcessingStateNone
		status.Warnings = []api.StateInfo{
			{Message: "Ready condition not found"},
		}
		return
	}

	if processing.Status == metav1.ConditionTrue {
		status.State = api.StateBlocked
		status.ProcessingState = api.ProcessingStateProcessing
		return
	}

	if processing.Reason == reasonBlocked {
		status.State = api.StateBlocked
		status.ProcessingState = api.ProcessingStateDone
		status.Warnings = []api.StateInfo{
			{Message: processing.Message},
		}
		return
	}

	if processing.Reason == reasonDone {
		status.ProcessingState = api.ProcessingStateDone
		if ready.Status == metav1.ConditionTrue {
			status.State = api.StateComplete
		} else {
			status.State = api.StateBlocked
			status.Warnings = []api.StateInfo{
				{Message: ready.Message},
			}
		}
		return
	}

	// Fallthrough: processing failed.
	status.State = api.StateInvalid
	status.ProcessingState = api.ProcessingStateFailed
	status.Errors = []api.StateInfo{
		{Message: processing.Message},
	}
}

// MapStatus maps a set of Kubernetes conditions to an api.Status.
func MapStatus(conditions []metav1.Condition, objectGeneration int64) api.Status {
	status := api.Status{
		ProcessingState: api.ProcessingStateNone,
		State:           api.StateNone,
	}
	fillStateInfo(conditions, objectGeneration, &status)
	return status
}

// CalculateOverallStatus collapses a State and ProcessingState into a single OverallStatus string.
func CalculateOverallStatus(s api.State, ps api.ProcessingState) string {
	if ps == api.ProcessingStateProcessing {
		return "processing"
	}
	if ps == api.ProcessingStateFailed {
		return "failed"
	}
	if s == api.StateBlocked {
		return "blocked"
	}
	if ps == api.ProcessingStatePending {
		return "pending"
	}
	if s == api.StateComplete && ps == api.ProcessingStateDone {
		return "complete"
	}
	return "none"
}
