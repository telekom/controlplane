// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"strings"

	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func mapExposure(in *roverv1.Exposure, out *api.Exposure) error {
	if in.Api != nil {
		if err := out.FromApiExposure(mapApiExposure(in.Api)); err != nil {
			return errors.Wrap(err, "failed to map api exposure")
		}

	} else if in.Event != nil {
		if err := out.FromEventExposure(mapEventExposure(in.Event)); err != nil {
			return errors.Wrap(err, "failed to map event exposure")
		}

	} else {
		return errors.Errorf("unknown exposure type: %s", in.Type())
	}
	return nil
}

func mapApiExposure(in *roverv1.ApiExposure) api.ApiExposure {
	apiExposure := api.ApiExposure{
		BasePath:   in.BasePath,
		Visibility: toApiVisibility(in.Visibility),
		Approval:   toApiApprovalStrategy(in.Approval.Strategy),
	}

	mapTrustedTeams(in, &apiExposure)

	if len(in.Upstreams) == 1 {
		apiExposure.Upstream = in.Upstreams[0].URL
	} else {
		apiExposure.LoadBalancing = api.LoadBalancing{
			Servers: []api.Server{},
		}
		for _, upstream := range in.Upstreams {
			apiExposure.LoadBalancing.Servers = append(apiExposure.LoadBalancing.Servers, api.Server{
				Upstream: upstream.URL,
				Weight:   upstream.Weight,
			})
		}
	}

	mapExposureSecurity(in, &apiExposure)
	mapExposureTransformation(in, &apiExposure)
	mapExposureTraffic(in, &apiExposure)

	return apiExposure
}

func mapEventExposure(in *roverv1.EventExposure) api.EventExposure {
	return api.EventExposure{
		EventType: in.EventType,
	}
}

func toApiVisibility(visibility roverv1.Visibility) api.Visibility {
	switch visibility {
	case roverv1.VisibilityWorld:
		return api.WORLD
	case roverv1.VisibilityZone:
		return api.ZONE
	case roverv1.VisibilityEnterprise:
		return api.ENTERPRISE
	default:
		return api.Visibility(strings.ToUpper(string(visibility)))
	}
}

func toApiApprovalStrategy(approval roverv1.ApprovalStrategy) api.ApprovalStrategy {
	switch approval {
	case roverv1.ApprovalStrategyAuto:
		return api.AUTO
	case roverv1.ApprovalStrategySimple:
		return api.SIMPLE
	case roverv1.ApprovalStrategyFourEyes:
		return api.FOUREYES
	default:
		return api.ApprovalStrategy(strings.ToUpper(string(approval)))
	}
}

func mapExposureSecurity(in *roverv1.ApiExposure, out *api.ApiExposure) {
	if in.Security == nil || in.Security.M2M == nil {
		return
	}

	m2m := in.Security.M2M
	if m2m.Basic != nil {
		basicAuth := api.BasicAuth{
			Username: m2m.Basic.Username,
			Password: m2m.Basic.Password,
		}
		out.Security = api.Security{}
		out.Security.FromBasicAuth(basicAuth)
		return
	}

	if m2m.ExternalIDP != nil {
		oauth2 := api.Oauth2{
			TokenEndpoint: m2m.ExternalIDP.TokenEndpoint,
			TokenRequest:  api.Oauth2TokenRequest(m2m.ExternalIDP.TokenRequest),
		}

		if grantType := api.GrantType(m2m.ExternalIDP.GrantType); grantType != "" {
			oauth2.GrantType = grantType
		}

		if m2m.ExternalIDP.Client != nil {
			oauth2.ClientId = m2m.ExternalIDP.Client.ClientId
			oauth2.ClientSecret = m2m.ExternalIDP.Client.ClientSecret
			oauth2.ClientKey = m2m.ExternalIDP.Client.ClientKey
		}

		if m2m.ExternalIDP.Basic != nil {
			oauth2.Username = m2m.ExternalIDP.Basic.Username
			oauth2.Password = m2m.ExternalIDP.Basic.Password
		}

		if len(m2m.Scopes) > 0 {
			oauth2.Scopes = m2m.Scopes
		}

		out.Security = api.Security{}
		out.Security.FromOauth2(oauth2)
		return
	}

	if len(m2m.Scopes) > 0 {
		oauth2 := api.Oauth2{
			Scopes: m2m.Scopes,
		}
		out.Security = api.Security{}
		out.Security.FromOauth2(oauth2)
	}
}

func mapExposureTransformation(in *roverv1.ApiExposure, out *api.ApiExposure) {
	if in.Transformation == nil || in.Transformation.Request.Headers.Remove == nil {
		return
	}

	if len(in.Transformation.Request.Headers.Remove) > 0 {
		out.RemoveHeaders = in.Transformation.Request.Headers.Remove
	}
}

func mapExposureTraffic(in *roverv1.ApiExposure, out *api.ApiExposure) {
	if in.Traffic.Failover != nil {
		out.Failover = api.Failover{
			Zones: in.Traffic.Failover.Zones,
		}
	}

	// todo: ratelimit (ignore for now until implementation is clear)
}

func mapTrustedTeams(in *roverv1.ApiExposure, out *api.ApiExposure) {
	if in.Approval.TrustedTeams == nil {
		return
	}

	out.TrustedTeams = make([]api.TrustedTeam, len(in.Approval.TrustedTeams))
	for i, team := range in.Approval.TrustedTeams {
		out.TrustedTeams[i] = api.TrustedTeam{
			Team: team.Group + "--" + team.Team,
		}
	}
}
