// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

// oauth2TokenRequestToCRD maps API tokenRequest values to CRD tokenRequest values.
var oauth2TokenRequestToCRD = map[string]roverv1.TokenRequestMethod{
	"body":   roverv1.TokenRequestClientSecretPost,
	"header": roverv1.TokenRequestClientSecretBasic,
	"basic":  roverv1.TokenRequestClientSecretBasic,
}

func tokenRequestAPIToCRD(value string) roverv1.TokenRequestMethod {
	if mapped, ok := oauth2TokenRequestToCRD[strings.ToLower(value)]; ok {
		return mapped
	}
	return roverv1.TokenRequestMethod(value)
}

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

		out.Event = mapEventExposure(eventExp)

	case "ai":
		aiExp, err := in.AsAiExposure()
		if err != nil {
			return errors.Wrap(err, "failed to convert to AiExposure")
		}

		out.Ai = mapAiExposure(aiExp)

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
				TokenRequest:  tokenRequestAPIToCRD(string(oauth2.TokenRequest)),
				GrantType:     roverv1.GrantType(strings.ToLower(string(oauth2.GrantType))),
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

func mapEventExposure(in api.EventExposure) *roverv1.EventExposure {
	out := &roverv1.EventExposure{
		EventType:  in.EventType,
		Visibility: toRoverVisibility(in.Visibility),
		Approval: roverv1.Approval{
			Strategy: toRoverApprovalStrategy(in.Approval),
		},
	}

	// Map trusted teams
	if in.TrustedTeams != nil {
		out.Approval.TrustedTeams = make([]roverv1.TrustedTeam, len(in.TrustedTeams))
		for i, team := range in.TrustedTeams {
			parts := strings.Split(team.Team, "--")
			if len(parts) != 2 {
				continue
			}
			out.Approval.TrustedTeams[i] = roverv1.TrustedTeam{
				Group: parts[0],
				Team:  parts[1],
			}
		}
	}

	// Map scopes
	if in.Scopes != nil {
		out.Scopes = make([]roverv1.EventScope, len(in.Scopes))
		for i, scope := range in.Scopes {
			out.Scopes[i] = roverv1.EventScope{
				Name: scope.Name,
			}
			if scope.Trigger.ResponseFilter != nil || scope.Trigger.SelectionFilter != nil || scope.Trigger.AdvancedSelectionFilter != nil {
				if t := mapEventTrigger(scope.Trigger); t != nil {
					out.Scopes[i].Trigger = *t
				}
			}
		}
	}

	// Map additional publisher IDs
	if in.AdditionalPublisherIds != nil {
		out.AdditionalPublisherIds = in.AdditionalPublisherIds
	}

	return out
}

func mapEventTrigger(in api.EventTrigger) *roverv1.EventTrigger {
	out := &roverv1.EventTrigger{}

	if in.ResponseFilter != nil {
		out.ResponseFilter = &roverv1.EventResponseFilter{
			Paths: in.ResponseFilter,
			Mode:  FuzzyMatchEventResponseFilterMode(string(in.ResponseFilterMode)),
		}
	}

	if in.SelectionFilter != nil || in.AdvancedSelectionFilter != nil {
		out.SelectionFilter = &roverv1.EventSelectionFilter{}
		if in.SelectionFilter != nil {
			out.SelectionFilter.Attributes = in.SelectionFilter
		}
		if in.AdvancedSelectionFilter != nil {
			jsonBytes, err := json.Marshal(in.AdvancedSelectionFilter)
			if err == nil {
				out.SelectionFilter.Expression = &apiextensionsv1.JSON{Raw: jsonBytes}
			}
		}
	}

	return out
}

func mapAiExposure(in api.AiExposure) *roverv1.AiExposure {
	out := &roverv1.AiExposure{}
	out.BasePath = in.BasePath
	out.Variant = roverv1.AiVariant(in.Variant)
	out.Visibility = toRoverVisibility(in.Visibility)
	out.Approval = roverv1.Approval{
		Strategy: toRoverApprovalStrategy(in.Approval),
	}
	mapAiTrustedTeams(in, out)

	if in.Upstream != "" {
		out.Upstreams = []roverv1.Upstream{
			{
				URL: in.Upstream,
			},
		}
	} else if len(in.LoadBalancing.Servers) > 0 {
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

	mapAiExposureSecurity(in, out)
	mapAiExposureTransformation(in, out)
	mapAiExposureTraffic(in, out)

	return out
}

func mapAiTrustedTeams(in api.AiExposure, out *roverv1.AiExposure) {
	if len(in.TrustedTeams) == 0 {
		return
	}

	out.Approval.TrustedTeams = make([]roverv1.TrustedTeam, len(in.TrustedTeams))
	for i, team := range in.TrustedTeams {
		parts := strings.Split(team.Team, "--")
		if len(parts) != 2 {
			continue
		}
		out.Approval.TrustedTeams[i] = roverv1.TrustedTeam{
			Group: parts[0],
			Team:  parts[1],
		}
	}
}

func mapAiExposureSecurity(in api.AiExposure, out *roverv1.AiExposure) {
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
			m2mSecurity.ExternalIDP = &roverv1.ExternalIdentityProvider{
				TokenEndpoint: oauth2.TokenEndpoint,
				TokenRequest:  tokenRequestAPIToCRD(string(oauth2.TokenRequest)),
				GrantType:     roverv1.GrantType(strings.ToLower(string(oauth2.GrantType))),
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
			m2mSecurity.Scopes = oauth2.Scopes
		}
	}

	if m2mSecurity.Basic != nil || m2mSecurity.ExternalIDP != nil || m2mSecurity.Scopes != nil {
		out.Security = &roverv1.Security{
			M2M: m2mSecurity,
		}
	}
}

func mapAiExposureTransformation(in api.AiExposure, out *roverv1.AiExposure) {
	if len(in.RemoveHeaders) == 0 {
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

func mapAiExposureTraffic(in api.AiExposure, out *roverv1.AiExposure) {
	if out.Traffic == nil {
		out.Traffic = &roverv1.Traffic{}
	}

	// Failover
	if len(in.Failover.Zones) > 0 {
		out.Traffic.Failover = &roverv1.ProviderFailover{
			Zones: in.Failover.Zones,
		}
	}

	// CircuitBreaker
	if in.CircuitBreaker.Enabled {
		out.Traffic.CircuitBreaker = &roverv1.CircuitBreaker{
			Enabled: in.CircuitBreaker.Enabled,
		}
	}

	// RateLimit
	if !reflect.ValueOf(in.RateLimit).IsZero() {
		mapAiRateLimit(in, out)
	}

	// Clean up empty traffic
	if out.Traffic.Failover == nil && out.Traffic.CircuitBreaker == nil && out.Traffic.RateLimit == nil {
		out.Traffic = nil
	}
}

func mapAiRateLimit(in api.AiExposure, out *roverv1.AiExposure) {
	rl := in.RateLimit
	hasProviderLimit := rl.Provider.Second != 0 || rl.Provider.Minute != 0 || rl.Provider.Hour != 0
	hasConsumerDefaultLimit := rl.ConsumerDefault.Second != 0 || rl.ConsumerDefault.Minute != 0 || rl.ConsumerDefault.Hour != 0
	hasConsumerOverrides := len(rl.Consumers) > 0

	if !hasProviderLimit && !hasConsumerDefaultLimit && !hasConsumerOverrides {
		return
	}

	if out.Traffic.RateLimit == nil {
		out.Traffic.RateLimit = &roverv1.RateLimit{}
	}

	if hasProviderLimit {
		out.Traffic.RateLimit.Provider = &roverv1.RateLimitConfig{
			Limits: &roverv1.Limits{
				Second: int(rl.Provider.Second),
				Minute: int(rl.Provider.Minute),
				Hour:   int(rl.Provider.Hour),
			},
			Options: roverv1.RateLimitOptions{
				FaultTolerant:     rl.Provider.FaultTolerant,
				HideClientHeaders: rl.Provider.HideClientHeaders,
			},
		}
	}

	if hasConsumerDefaultLimit || hasConsumerOverrides {
		out.Traffic.RateLimit.Consumers = &roverv1.ConsumerRateLimits{}

		if hasConsumerDefaultLimit {
			out.Traffic.RateLimit.Consumers.Default = &roverv1.ConsumerRateLimitDefaults{
				Limits: roverv1.Limits{
					Second: int(rl.ConsumerDefault.Second),
					Minute: int(rl.ConsumerDefault.Minute),
					Hour:   int(rl.ConsumerDefault.Hour),
				},
			}
		}

		if hasConsumerOverrides {
			overrides := make([]roverv1.ConsumerRateLimitOverrides, len(rl.Consumers))
			for i, consumer := range rl.Consumers {
				overrides[i] = roverv1.ConsumerRateLimitOverrides{
					Consumer: consumer.Id,
					Limits: roverv1.Limits{
						Second: int(consumer.Second),
						Minute: int(consumer.Minute),
						Hour:   int(consumer.Hour),
					},
				}
			}
			out.Traffic.RateLimit.Consumers.Overrides = overrides
		}
	}
}
