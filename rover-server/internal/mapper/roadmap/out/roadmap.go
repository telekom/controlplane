// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"strings"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	statusmapper "github.com/telekom/controlplane/rover-server/internal/mapper/status"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// MapResponse maps a Roadmap CRD and its decoded items to the API response.
func MapResponse(roadmap *roverv1.Roadmap, items []api.ApiRoadmapItem) api.ApiRoadmapResponse {
	basePath := ""
	if roadmap.Annotations != nil {
		basePath = roadmap.Annotations["rover.cp.ei.telekom.de/basePath"]
	}
	if basePath == "" {
		// Fallback: try to derive from specification name
		basePath = "/" + strings.ReplaceAll(roadmap.Spec.SpecificationRef.Name, "-", "/")
	}

	return api.ApiRoadmapResponse{
		BasePath: basePath,
		Id:       mapper.MakeResourceId(roadmap),
		Name:     roadmap.Name,
		Items:    items,
		Status:   statusmapper.MapStatus(roadmap.GetConditions(), roadmap.GetGeneration()),
	}
}
