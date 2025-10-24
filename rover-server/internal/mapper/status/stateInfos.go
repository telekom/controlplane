// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"

	v1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

// GetAllStateInfos retrieves all state information for a given Rover resource including all sub resources.
// It uses GetAllProblems and maps problems to state information.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose state information is being retrieved.
//
// Returns:
// - *[]api.StateInfo: A pointer to a slice of state information.
func GetAllStateInfos(ctx context.Context, rover *v1.Rover) []api.StateInfo {
	return mapProblemsToStateInfos(GetAllProblems(ctx, rover))
}

// mapProblemsToStateInfos maps a slice of problems to a slice of state information.
//
// Parameters:
// - problems: A pointer to a slice of problems.
//
// Returns:
// - *[]api.StateInfo: A pointer to a slice of state information.
func mapProblemsToStateInfos(problems []api.Problem) []api.StateInfo {
	var stateInfos = []api.StateInfo{}
	if problems == nil {
		return stateInfos
	}

	for _, problem := range problems {
		stateInfos = append(stateInfos, api.StateInfo{
			Message: problem.Message,
			Cause:   problem.Cause,
		})
	}
	return stateInfos
}

// AppendStateInfos appends new state information to an existing slice of state information.
//
// Parameters:
// - states: A pointer to a slice of existing state information.
// - newStates: A pointer to a slice of new state information to be appended.
//
// Returns:
// - []api.StateInfo: The updated slice with new states appended
func AppendStateInfos(states []api.StateInfo, newStates []api.StateInfo) []api.StateInfo {
	if newStates != nil {
		return append(states, newStates...)
	}
	return states
}
