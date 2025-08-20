// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"strings"

	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func mapExposure(in *api.Exposure, out *roverv1.Exposure) error {
	expType, err := in.Discriminator()
	if err != nil {
		return errors.Wrap(err, "failed to get exposure type")
	}
	switch expType {
	case "api":
		apiExp, err := in.AsApiExposure()
		if err != nil {
			return errors.Wrap(err, "failed to convert to ApiExposure")
		}

		out.Api = mapApiExposure(apiExp)

	case "event":
		eventExp, err := in.AsEventExposure()
		if err != nil {
			return errors.Wrap(err, "failed to convert to EventExposure")
		}

		out.Event = &roverv1.EventExposure{
			EventType: eventExp.EventType,
		}

	default:
		return errors.Errorf("unknown exposure type: %s", expType)
	}

	return nil
}

func mapApiExposure(in api.ApiExposure) *roverv1.ApiExposure {
	out := &roverv1.ApiExposure{}
	out.BasePath = in.BasePath
	out.Visibility = toRoverVisibility(in.Visibility)
	out.Approval = roverv1.Approval{
		Strategy: toRoverApprovalStrategy(in.Approval),
	}
	mapTrustedTeams(in, out)

	if in.Upstream != "" {
		out.Upstreams = []roverv1.Upstream{
			{
				URL: in.Upstream,
			},
		}
	} else {
		out.Upstreams = make([]roverv1.Upstream, len(in.LoadBalancing.Servers))
		for i, server := range in.LoadBalancing.Servers {
			out.Upstreams[i] = roverv1.Upstream{
				URL: server.Upstream,
			}
			if server.Weight != 0 {
				out.Upstreams[i].Weight = server.Weight
			}
		}
	}

	mapExposureSecurity(in, out)
	mapExposureTransformation(in, out)
	mapExposureTraffic(in, out)

	return out
}

func toRoverVisibility(visibility api.Visibility) roverv1.Visibility {
	switch visibility {
	case api.WORLD:
		return roverv1.VisibilityWorld
	case api.ZONE:
		return roverv1.VisibilityZone
	case api.ENTERPRISE:
		return roverv1.VisibilityEnterprise
	default:
		return roverv1.Visibility(cases.Title(language.Und).String(strings.ToLower(string(visibility))))
	}
}

func toRoverApprovalStrategy(approval api.ApprovalStrategy) roverv1.ApprovalStrategy {
	switch approval {
	case api.AUTO:
		return roverv1.ApprovalStrategyAuto
	case api.SIMPLE:
		return roverv1.ApprovalStrategySimple
	case api.FOUREYES:
		return roverv1.ApprovalStrategyFourEyes
	default:
		return roverv1.ApprovalStrategy(cases.Title(language.Und).String(strings.ToLower(string(approval))))
	}
}

func mapExposureSecurity(in api.ApiExposure, out *roverv1.ApiExposure) {
	m2mSecurity := &roverv1.Machine2MachineAuthentication{}

	secType, err := in.Security.Discriminator()
	if err != nil {
		return
	}
	switch secType {
	case "basicAuth":
		basicAuth, err := in.Security.AsBasicAuth()
		if err != nil {
			return
		}
		m2mSecurity.Basic = &roverv1.BasicAuthCredentials{
			Username: basicAuth.Username,
			Password: basicAuth.Password,
		}

	case "oauth2":
		oauth2, err := in.Security.AsOauth2()
		if err != nil {
			return
		}

		if oauth2.TokenEndpoint != "" {
			// external-idp
			m2mSecurity.ExternalIDP = &roverv1.ExternalIdentityProvider{
				TokenEndpoint: oauth2.TokenEndpoint,
				TokenRequest:  string(oauth2.TokenRequest),
				GrantType:     strings.ToLower(string(oauth2.GrantType)),
			}
			if oauth2.ClientId != "" {
				m2mSecurity.ExternalIDP.Client = &roverv1.OAuth2ClientCredentials{
					ClientId:     oauth2.ClientId,
					ClientSecret: oauth2.ClientSecret,
					ClientKey:    oauth2.ClientKey,
				}
			}
			if oauth2.Username != "" {
				m2mSecurity.ExternalIDP.Basic = &roverv1.BasicAuthCredentials{
					Username: oauth2.Username,
					Password: oauth2.Password,
				}
			}
		}
		if oauth2.Scopes != nil {
			// scopes
			m2mSecurity.Scopes = oauth2.Scopes
		}
	}

	if m2mSecurity.Basic != nil || m2mSecurity.ExternalIDP != nil || m2mSecurity.Scopes != nil {
		out.Security = &roverv1.Security{
			M2M: m2mSecurity,
		}
	}
}

func mapExposureTransformation(in api.ApiExposure, out *roverv1.ApiExposure) {
	if in.RemoveHeaders == nil {
		return
	}
	out.Transformation = &roverv1.Transformation{
		Request: roverv1.RequestResponseTransformation{
			Headers: roverv1.HeaderTransformation{
				Remove: in.RemoveHeaders,
			},
		},
	}

}

func mapTrustedTeams(in api.ApiExposure, out *roverv1.ApiExposure) {
	if in.TrustedTeams == nil {
		return
	}

	out.Approval.TrustedTeams = make([]roverv1.TrustedTeam, len(in.TrustedTeams))
	for i, team := range in.TrustedTeams {
		parts := strings.Split(team.Team, "--")
		if len(parts) != 2 {
			continue // invalid team format, skip
		}
		out.Approval.TrustedTeams[i] = roverv1.TrustedTeam{
			Group: parts[0],
			Team:  parts[1],
		}
	}
}
