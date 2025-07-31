// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func TestCalculateOverallStatus_ProcessingStateProcessing_ReturnsProcessing(t *testing.T) {
	result := CalculateOverallStatus(api.Complete, api.ProcessingStateProcessing)
	assert.Equal(t, api.OverallStatusProcessing, result)
}

func TestCalculateOverallStatus_ProcessingStateFailed_ReturnsFailed(t *testing.T) {
	result := CalculateOverallStatus(api.Complete, api.ProcessingStateFailed)
	assert.Equal(t, api.OverallStatusFailed, result)
}

func TestCalculateOverallStatus_StateBlocked_ReturnsBlocked(t *testing.T) {
	result := CalculateOverallStatus(api.Blocked, api.ProcessingStatePending)
	assert.Equal(t, api.OverallStatusBlocked, result)
}

func TestCalculateOverallStatus_CompleteAndDone_ReturnsComplete(t *testing.T) {
	result := CalculateOverallStatus(api.Complete, api.ProcessingStateDone)
	assert.Equal(t, api.OverallStatusComplete, result)
}

func TestCalculateOverallStatus_UnknownState_ReturnsNone(t *testing.T) {
	result := CalculateOverallStatus("unknown", api.ProcessingStateNone)
	assert.Equal(t, api.OverallStatusNone, result)
}

func TestCompareAndReturn_FailedTakesPrecedence(t *testing.T) {
	result := CompareAndReturn(api.OverallStatusProcessing, api.OverallStatusFailed)
	assert.Equal(t, api.OverallStatusFailed, result)
}

func TestCompareAndReturn_BlockedTakesPrecedenceOverProcessing(t *testing.T) {
	result := CompareAndReturn(api.OverallStatusProcessing, api.OverallStatusBlocked)
	assert.Equal(t, api.OverallStatusBlocked, result)
}

func TestCompareAndReturn_ProcessingTakesPrecedenceOverPending(t *testing.T) {
	result := CompareAndReturn(api.OverallStatusPending, api.OverallStatusProcessing)
	assert.Equal(t, api.OverallStatusProcessing, result)
}

func TestCompareAndReturn_CompleteWhenBothAreComplete(t *testing.T) {
	result := CompareAndReturn(api.OverallStatusComplete, api.OverallStatusComplete)
	assert.Equal(t, api.OverallStatusComplete, result)
}

func TestCompareAndReturn_NoneWhenNoHigherStatus(t *testing.T) {
	result := CompareAndReturn(api.OverallStatusNone, api.OverallStatusNone)
	assert.Equal(t, api.OverallStatusNone, result)
}
