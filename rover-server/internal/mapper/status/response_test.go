// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/rover-server/internal/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMapResponse_WithValidConditions(t *testing.T) {
	// Test with processing condition = Done and ready condition = True
	conditions := []metav1.Condition{
		{
			Type:   condition.ConditionTypeProcessing,
			Status: metav1.ConditionFalse,
			Reason: "Done",
		},
		{
			Type:   condition.ConditionTypeReady,
			Status: metav1.ConditionTrue,
		},
	}

	response, err := MapResponse(conditions)

	// Verify the response
	assert.NoError(t, err)
	assert.Equal(t, api.Complete, response.State)
	assert.Equal(t, api.ProcessingStateDone, response.ProcessingState)
	assert.Equal(t, api.OverallStatusComplete, response.OverallStatus)
}

func TestMapResponse_WithBlockedConditions(t *testing.T) {
	// Test with processing condition = Blocked
	conditions := []metav1.Condition{
		{
			Type:    condition.ConditionTypeProcessing,
			Status:  metav1.ConditionFalse,
			Reason:  "Blocked",
			Message: "Resource is blocked",
		},
		{
			Type:   condition.ConditionTypeReady,
			Status: metav1.ConditionFalse,
		},
	}

	response, err := MapResponse(conditions)

	// Verify the response
	assert.NoError(t, err)
	assert.Equal(t, api.Blocked, response.State)
	assert.Equal(t, api.ProcessingStateDone, response.ProcessingState)
	assert.Equal(t, api.OverallStatusBlocked, response.OverallStatus)
}

func TestMapResponse_WithEmptyConditions(t *testing.T) {
	// Test with empty conditions
	var conditions []metav1.Condition

	response, err := MapResponse(conditions)

	// Verify the response
	assert.NoError(t, err)
	assert.Equal(t, api.None, response.State)
	assert.Equal(t, api.ProcessingStateNone, response.ProcessingState)
	assert.Equal(t, api.OverallStatusNone, response.OverallStatus)
}
