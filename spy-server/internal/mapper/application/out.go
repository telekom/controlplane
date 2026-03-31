// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"

	"github.com/telekom/controlplane/spy-server/internal/api"
	"github.com/telekom/controlplane/spy-server/internal/mapper"
	"github.com/telekom/controlplane/spy-server/internal/mapper/status"
)

// MapResponse maps an Application CRD to an ApplicationResponse.
func MapResponse(in *applicationv1.Application) api.ApplicationResponse {
	nsInfo := mapper.ParseNamespace(in.GetNamespace())

	resp := api.ApplicationResponse{
		Id:   mapper.MakeResourceName(in), // <group>--<team>--<appName>
		Name: in.GetName(),
		Team: api.Team{
			Hub:      nsInfo.Group,
			Name:     in.Spec.Team,
			Email:    openapi_types.Email(in.Spec.TeamEmail),
			Category: "", // Deferred — not in CRD
		},
		Zone:   in.Spec.Zone.Name,
		Status: status.MapStatus(in.GetConditions(), in.GetGeneration()),
		// icto, apid, psiid: left empty (deferred — not in CRD)
	}

	mapSecurity(in, &resp)
	return resp
}

// mapSecurity maps the CRD Security.IpRestrictions to the API Security model.
func mapSecurity(in *applicationv1.Application, out *api.ApplicationResponse) {
	if in.Spec.Security == nil || in.Spec.Security.IpRestrictions == nil {
		return
	}

	out.Security = api.Security{
		IpRestrictions: api.IpRestrictions{
			Allow: in.Spec.Security.IpRestrictions.Allow,
		},
	}
}
