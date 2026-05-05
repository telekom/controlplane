// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/discovery-server/internal/api"
)

// MapResponse maps the status of a resource to a ResourceStatusResponse.
func MapResponse(obj ctypes.Object) api.ResourceStatusResponse {
	status := MapStatus(obj.GetConditions(), obj.GetGeneration())

	processing := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
	var processedAtTime time.Time
	if processing != nil {
		processedAtTime = processing.LastTransitionTime.Time
	}

	return api.ResourceStatusResponse{
		CreatedAt:       obj.GetCreationTimestamp().Time,
		ProcessedAt:     processedAtTime,
		State:           status.State,
		ProcessingState: status.ProcessingState,
		OverallStatus:   CalculateOverallStatus(status.State, status.ProcessingState),
	}
}
