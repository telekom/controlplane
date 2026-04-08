// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventtype

import (
	eventv1 "github.com/telekom/controlplane/event/api/v1"

	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	"github.com/telekom/controlplane/discovery-server/internal/mapper/status"
)

const (
	// EventTypeActiveJsonPath is the JSON path to the "active" field in the EventType CRD status.
	EventTypeActiveJsonPath = "status.active"
)

// MapResponse maps an EventType CRD to an EventTypeResponse.
func MapResponse(in *eventv1.EventType) api.EventTypeResponse {
	return api.EventTypeResponse{
		Name:          mapper.MakeResourceName(in),
		Id:            mapper.MakeResourceId(in),
		Type:          in.Spec.Type,
		Version:       in.Spec.Version,
		Description:   in.Spec.Description,
		Specification: in.Spec.Specification,
		Active:        in.Status.Active,
		Status:        status.MapStatus(in.GetConditions(), in.GetGeneration()),
	}
}
