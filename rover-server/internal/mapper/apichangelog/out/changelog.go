// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	statusmapper "github.com/telekom/controlplane/rover-server/internal/mapper/status"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// MapResponse maps an ApiChangelog CRD and its decoded items to the API response.
func MapResponse(changelog *roverv1.ApiChangelog, items []api.ApiChangelogItem) api.ApiChangelogResponse {
	basePath := ""
	if changelog.Annotations != nil {
		basePath = changelog.Annotations[config.BuildLabelKey("basePath")]
	}
	if basePath == "" {
		// Fallback: try to derive from specification name
		basePath = "/" + strings.ReplaceAll(changelog.Spec.SpecificationRef.Name, "-", "/")
	}

	return api.ApiChangelogResponse{
		BasePath: basePath,
		Id:       mapper.MakeResourceId(changelog),
		Name:     changelog.Name,
		Items:    items,
		Status:   statusmapper.MapStatus(changelog.GetConditions(), changelog.GetGeneration()),
	}
}
