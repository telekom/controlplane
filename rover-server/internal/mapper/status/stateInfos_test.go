// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func TestAppendStateInfos_AppendsNewStateInfos(t *testing.T) {
	existingStateInfos := []api.StateInfo{
		{Message: "Existing state info"},
	}
	newStateInfos := []api.StateInfo{
		{Message: "New state info"},
	}

	result := AppendStateInfos(existingStateInfos, newStateInfos)

	assert.Len(t, result, 2)
	assert.Equal(t, "Existing state info", result[0].Message)
	assert.Equal(t, "New state info", result[1].Message)
}

func TestAppendStateInfos_NoNewStateInfos(t *testing.T) {
	existingStateInfos := []api.StateInfo{
		{Message: "Existing state info"},
	}

	result := AppendStateInfos(existingStateInfos, nil)

	assert.Len(t, result, 1)
	assert.Equal(t, "Existing state info", result[0].Message)
}

func TestMapProblemsToStateInfos_ReturnsMappedStateInfos(t *testing.T) {
	problems := []api.Problem{
		{Message: "Problem 1", Context: "Context 1", Cause: "Cause 1"},
		{Message: "Problem 2", Context: "Context 2", Cause: "Cause 2"},
	}
	expectedStateInfos := []api.StateInfo{
		{Message: "Problem 1", Cause: "Context 1, Cause: Cause 1"},
		{Message: "Problem 2", Cause: "Context 2, Cause: Cause 2"},
	}

	stateInfos := mapProblemsToStateInfos(problems)

	assert.Equal(t, expectedStateInfos, stateInfos)
}

func TestMapProblemsToStateInfos_EmptyProblems(t *testing.T) {
	problems := []api.Problem{}
	expectedStateInfos := []api.StateInfo{}

	stateInfos := mapProblemsToStateInfos(problems)

	assert.Equal(t, expectedStateInfos, stateInfos)
}

func TestMapProblemsToStateInfos_NilProblems(t *testing.T) {
	var problems []api.Problem
	expectedStateInfos := []api.StateInfo{}

	stateInfos := mapProblemsToStateInfos(problems)

	assert.Equal(t, expectedStateInfos, stateInfos)
}
