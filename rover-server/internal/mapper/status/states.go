// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"github.com/telekom/controlplane/rover-server/internal/api"
)

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

// CompareAndReturn compares two overall statuses and returns the higher one
func CompareAndReturn(a, b api.OverallStatus) api.OverallStatus {
	if a == api.OverallStatusFailed || b == api.OverallStatusFailed {
		return api.OverallStatusFailed
	}

	if a == api.OverallStatusBlocked || b == api.OverallStatusBlocked {
		return api.OverallStatusBlocked
	}

	if a == api.OverallStatusProcessing || b == api.OverallStatusProcessing {
		return api.OverallStatusProcessing
	}

	if a == api.OverallStatusPending || b == api.OverallStatusPending {
		return api.OverallStatusPending
	}

	if a == api.OverallStatusComplete && b == api.OverallStatusComplete {
		return api.OverallStatusComplete
	}

	return api.OverallStatusNone
}
