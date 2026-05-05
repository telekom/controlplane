// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"strings"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	"github.com/telekom/controlplane/discovery-server/internal/mapper/status"
)

// tokenRequestCRDToAPI converts CRD tokenRequest values to discovery-server API enum values.
func tokenRequestCRDToAPI(value string) api.OAuth2TokenRequest {
	switch strings.ToLower(value) {
	case "client_secret_basic":
		return api.Header
	case "client_secret_post":
		return api.Body
	default:
		return api.OAuth2TokenRequest(value)
	}
}

// MapResponse maps an ApiExposure CRD to an ApiExposureResponse.
func MapResponse(in *apiv1.ApiExposure) api.ApiExposureResponse {
	resp := api.ApiExposureResponse{
		Name:       mapper.MakeResourceName(in),
		BasePath:   in.Spec.ApiBasePath,
		Visibility: toAPIVisibility(in.Spec.Visibility),
		Approval:   toAPIApprovalStrategy(in.Spec.Approval.Strategy),
		Zone:       in.Spec.Zone.Name,
		Variant:    api.ApiExposureResponseVariantDEFAULT,
		Status:     status.MapStatus(in.GetConditions(), in.GetGeneration()),
	}

	mapUpstreams(in, &resp)
	mapTrustedTeams(in, &resp)
	mapSecurity(in, &resp)
	mapTransformation(in, &resp)
	mapTraffic(in, &resp)

	return resp
}

func mapUpstreams(in *apiv1.ApiExposure, out *api.ApiExposureResponse) {
	if len(in.Spec.Upstreams) == 1 {
		out.Upstream = in.Spec.Upstreams[0].Url
	} else if len(in.Spec.Upstreams) > 1 {
		servers := make([]api.Server, len(in.Spec.Upstreams))
		for i, u := range in.Spec.Upstreams {
			servers[i] = api.Server{
				Upstream: u.Url,
				Weight:   u.Weight,
			}
		}
		out.LoadBalancing = api.LoadBalancing{Servers: servers}
	}
}

func mapTrustedTeams(in *apiv1.ApiExposure, out *api.ApiExposureResponse) {
	if len(in.Spec.Approval.TrustedTeams) == 0 {
		return
	}
	teams := make([]api.TeamRef, len(in.Spec.Approval.TrustedTeams))
	for i, t := range in.Spec.Approval.TrustedTeams {
		group, team := mapper.SplitTeamName(t)
		teams[i] = api.TeamRef{Hub: group, Name: team}
	}
	out.TrustedTeams = teams
}

func mapSecurity(in *apiv1.ApiExposure, out *api.ApiExposureResponse) {
	if in.Spec.Security == nil || in.Spec.Security.M2M == nil {
		return
	}

	m2m := in.Spec.Security.M2M

	if m2m.Basic != nil {
		basicAuth := api.BasicAuth{
			Username: m2m.Basic.Username,
			Password: m2m.Basic.Password,
		}
		out.Security = api.SubscriberSecurity{}
		out.Security.FromBasicAuth(basicAuth) //nolint:errcheck,gosec // union setter only fails on JSON marshal of simple struct
		return
	}

	if m2m.ExternalIDP != nil {
		oauth2 := api.OAuth2{
			TokenEndpoint: m2m.ExternalIDP.TokenEndpoint,
			TokenRequest:  tokenRequestCRDToAPI(m2m.ExternalIDP.TokenRequest),
			GrantType:     m2m.ExternalIDP.GrantType,
		}

		if m2m.ExternalIDP.Client != nil {
			oauth2.ClientId = m2m.ExternalIDP.Client.ClientId
			oauth2.ClientSecret = m2m.ExternalIDP.Client.ClientSecret
		}

		if m2m.ExternalIDP.Basic != nil {
			oauth2.Username = m2m.ExternalIDP.Basic.Username
			oauth2.Password = m2m.ExternalIDP.Basic.Password
		}

		if len(m2m.Scopes) > 0 {
			oauth2.Scopes = m2m.Scopes
		}

		out.Security = api.SubscriberSecurity{}
		out.Security.FromOAuth2(oauth2) //nolint:errcheck,gosec // union setter only fails on JSON marshal of simple struct
		return
	}

	// Scopes only → represented as OAuth2 with just scopes.
	if len(m2m.Scopes) > 0 {
		oauth2 := api.OAuth2{
			Scopes: m2m.Scopes,
		}
		out.Security = api.SubscriberSecurity{}
		out.Security.FromOAuth2(oauth2) //nolint:errcheck,gosec // union setter only fails on JSON marshal of simple struct
	}
}

func mapTransformation(in *apiv1.ApiExposure, out *api.ApiExposureResponse) {
	if in.Spec.Transformation == nil {
		return
	}
	if len(in.Spec.Transformation.Request.Headers.Remove) > 0 {
		out.RemoveHeaders = in.Spec.Transformation.Request.Headers.Remove
	}
}

func mapTraffic(in *apiv1.ApiExposure, out *api.ApiExposureResponse) {
	// Failover
	if in.Spec.Traffic.Failover != nil && len(in.Spec.Traffic.Failover.Zones) > 0 {
		out.Failover = api.ExposureFailover{
			Zone: in.Spec.Traffic.Failover.Zones[0].Name,
		}
	}

	// Rate limit (provider level only — the OpenAPI spec exposes a flat RateLimit)
	if in.Spec.Traffic.RateLimit != nil && in.Spec.Traffic.RateLimit.Provider != nil {
		limits := in.Spec.Traffic.RateLimit.Provider.Limits
		out.RateLimit = api.RateLimit{
			Second:            int32(limits.Second), //nolint:gosec // G115: rate limit values are validated by kubebuilder (>=0, practically small)
			Minute:            int32(limits.Minute), //nolint:gosec // G115: rate limit values are validated by kubebuilder (>=0, practically small)
			Hour:              int32(limits.Hour),   //nolint:gosec // G115: rate limit values are validated by kubebuilder (>=0, practically small)
			FaultTolerant:     in.Spec.Traffic.RateLimit.Provider.Options.FaultTolerant,
			HideClientHeaders: in.Spec.Traffic.RateLimit.Provider.Options.HideClientHeaders,
		}
	}
}

func toAPIVisibility(v apiv1.Visibility) api.Visibility {
	switch v {
	case apiv1.VisibilityWorld:
		return api.WORLD
	case apiv1.VisibilityZone:
		return api.ZONE
	case apiv1.VisibilityEnterprise:
		return api.ENTERPRISE
	default:
		return api.Visibility(strings.ToUpper(string(v)))
	}
}

func toAPIApprovalStrategy(s apiv1.ApprovalStrategy) api.ApprovalStrategy {
	switch s {
	case apiv1.ApprovalStrategyAuto:
		return api.AUTO
	case apiv1.ApprovalStrategySimple:
		return api.SIMPLE
	case apiv1.ApprovalStrategyFourEyes:
		return api.FOUREYES
	default:
		return api.ApprovalStrategy(strings.ToUpper(string(s)))
	}
}
