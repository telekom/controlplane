// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"

	v1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	roverStore "github.com/telekom/controlplane/rover-server/pkg/store"
)

// GetAllProblems retrieves all problems for a given Rover resource including all sub resources.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose problems are being retrieved.
//
// Returns:
// - []api.Problem: A slice of problems.
func GetAllProblems(ctx context.Context, rover *v1.Rover) []api.Problem {
	var problems = []api.Problem{}
	messages, _ := getAllProblemsInApiSubscriptions(ctx, rover)
	problems = append(problems, messages...)

	messages, _ = getAllProblemsInApiExposures(ctx, rover)
	problems = append(problems, messages...)

	messages, _ = getAllProblemsInApplications(ctx, rover)
	problems = append(problems, messages...)

	return problems
}

// getAllProblemsInApiSubscriptions retrieves all problems in API subscriptions for a given Rover resource.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose API subscription problems are being retrieved.
//
// Returns:
// - []api.Problem: A slice of problems.
// - error: Any error encountered during the retrieval process.
func getAllProblemsInApiSubscriptions(ctx context.Context, rover *v1.Rover) ([]api.Problem, error) {
	return GetAllProblemsInSubResource(ctx, rover, roverStore.ApiSubscriptionStore)
}

// getAllProblemsInApiExposures retrieves all problems in API exposures for a given Rover resource.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose API exposure problems are being retrieved.
//
// Returns:
// - []api.Problem: A slice of problems.
// - error: Any error encountered during the retrieval process.
func getAllProblemsInApiExposures(ctx context.Context, rover *v1.Rover) ([]api.Problem, error) {
	return GetAllProblemsInSubResource(ctx, rover, roverStore.ApiExposureStore)
}

// getAllProblemsInApplications retrieves all problems in applications for a given Rover resource.
//
// Parameters:
// - ctx: The context for the operation.
// - rover: The Rover resource whose application problems are being retrieved.
//
// Returns:
// - []api.Problem: A slice of problems.
// - error: Any error encountered during the retrieval process.
func getAllProblemsInApplications(ctx context.Context, rover *v1.Rover) ([]api.Problem, error) {
	return GetAllProblemsInSubResource(ctx, rover, roverStore.ApplicationStore)
}
