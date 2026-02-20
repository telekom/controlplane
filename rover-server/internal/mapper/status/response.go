// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/rover-server/internal/api"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// MapResponse maps the status of a generic resource to a ResourceStatusResponse.
func MapResponse(ctx context.Context, obj types.Object) (api.ResourceStatusResponse, error) {
	if obj == nil {
		return api.ResourceStatusResponse{}, errors.New("input object is nil")
	}
	status := MapStatus(obj.GetConditions())
	processing := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
	var processedAt time.Time
	if processing != nil {
		processedAt = processing.LastTransitionTime.Time
	}

	var problems = []api.Problem{}

	if rover, ok := obj.(*v1.Rover); ok && status.State != api.Complete {
		// Load all sub resources and check for conditions with error state
		problems = append(problems, GetAllProblems(ctx, rover)...)
	}

	return api.ResourceStatusResponse{
		CreatedAt:       obj.GetCreationTimestamp().Time,
		ProcessedAt:     processedAt,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
		Errors:          problems,
	}, nil
}
