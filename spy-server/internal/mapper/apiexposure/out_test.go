// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"testing"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/spy-server/internal/api"
)

func TestMapResponse(t *testing.T) {
	t.Parallel()

	in := &apiv1.ApiExposure{}
	in.Name = "my-app--eni-distr-v1"
	in.Namespace = "poc--eni--hyperion"
	in.Spec.ApiBasePath = "/eni/distr/v1"
	in.Spec.Visibility = apiv1.VisibilityWorld
	in.Spec.Approval = apiv1.Approval{Strategy: apiv1.ApprovalStrategySimple}
	in.Spec.Zone = ctypes.ObjectRef{Name: "dataplane1"}
	in.Spec.Upstreams = []apiv1.Upstream{{Url: "https://httpbin.org/anything", Weight: 100}}
	in.Spec.Approval.TrustedTeams = []string{"eni--hyperion"}
	in.Spec.Transformation = &apiv1.Transformation{
		Request: apiv1.RequestResponseTransformation{
			Headers: apiv1.HeaderTransformation{Remove: []string{"x-internal"}},
		},
	}
	in.Spec.Traffic.Failover = &apiv1.Failover{Zones: []ctypes.ObjectRef{{Name: "dataplane2"}}}
	in.Spec.Traffic.RateLimit = &apiv1.RateLimit{
		Provider: &apiv1.RateLimitConfig{
			Limits: apiv1.Limits{Second: 10, Minute: 100, Hour: 1000},
			Options: apiv1.RateLimitOptions{
				FaultTolerant:     true,
				HideClientHeaders: true,
			},
		},
	}

	resp := MapResponse(in)

	if resp.Name != "my-app--eni-distr-v1" {
		t.Fatalf("unexpected name: %q", resp.Name)
	}
	if resp.Upstream != "https://httpbin.org/anything" {
		t.Fatalf("unexpected upstream: %q", resp.Upstream)
	}
	if resp.Visibility != api.WORLD {
		t.Fatalf("unexpected visibility: %q", resp.Visibility)
	}
	if resp.Approval != api.SIMPLE {
		t.Fatalf("unexpected approval strategy: %q", resp.Approval)
	}
	if len(resp.TrustedTeams) != 1 || resp.TrustedTeams[0].Hub != "eni" || resp.TrustedTeams[0].Name != "hyperion" {
		t.Fatalf("unexpected trusted teams mapping: %#v", resp.TrustedTeams)
	}
	if len(resp.RemoveHeaders) != 1 || resp.RemoveHeaders[0] != "x-internal" {
		t.Fatalf("unexpected removeHeaders mapping: %#v", resp.RemoveHeaders)
	}
	if resp.Failover.Zone != "dataplane2" {
		t.Fatalf("unexpected failover zone: %#v", resp.Failover)
	}
	if resp.RateLimit.Second != 10 || resp.RateLimit.Minute != 100 || resp.RateLimit.Hour != 1000 {
		t.Fatalf("unexpected rate limit mapping: %#v", resp.RateLimit)
	}
}

func TestMapUpstreams_LoadBalancing(t *testing.T) {
	t.Parallel()

	in := &apiv1.ApiExposure{}
	in.Spec.Upstreams = []apiv1.Upstream{
		{Url: "https://one.example", Weight: 30},
		{Url: "https://two.example", Weight: 70},
	}

	out := api.ApiExposureResponse{}
	mapUpstreams(in, &out)

	if out.Upstream != "" {
		t.Fatalf("expected empty upstream for load balancing case, got %q", out.Upstream)
	}
	if len(out.LoadBalancing.Servers) != 2 {
		t.Fatalf("expected 2 load balancing servers, got %#v", out.LoadBalancing.Servers)
	}
}

func TestMapTrustedTeams_Empty(t *testing.T) {
	t.Parallel()

	in := &apiv1.ApiExposure{}
	out := api.ApiExposureResponse{}
	mapTrustedTeams(in, &out)
	if len(out.TrustedTeams) != 0 {
		t.Fatalf("expected no trusted teams, got %#v", out.TrustedTeams)
	}
}

func TestMapSecurity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(in *apiv1.ApiExposure)
		assert func(t *testing.T, out api.ApiExposureResponse)
	}{
		{
			name: "no security",
			setup: func(in *apiv1.ApiExposure) {
				in.Spec.Security = nil
			},
			assert: func(t *testing.T, out api.ApiExposureResponse) {
				t.Helper()
				// Neither variant should be set when spec has no security.
				_, errOAuth := out.Security.AsOAuth2()
				_, errBasic := out.Security.AsBasicAuth()
				if errOAuth == nil || errBasic == nil {
					t.Fatalf("expected no security to be set, but got a valid variant")
				}
			},
		},
		{
			name: "basic auth",
			setup: func(in *apiv1.ApiExposure) {
				in.Spec.Security = &apiv1.Security{M2M: &apiv1.Machine2MachineAuthentication{Basic: &apiv1.BasicAuthCredentials{Username: "u", Password: "p"}}}
			},
			assert: func(t *testing.T, out api.ApiExposureResponse) {
				t.Helper()
				got, err := out.Security.AsBasicAuth()
				if err != nil {
					t.Fatalf("expected basic auth security, got err: %v", err)
				}
				if got.Username != "u" || got.Password != "p" {
					t.Fatalf("unexpected basic auth mapping: %#v", got)
				}
			},
		},
		{
			name: "external idp oauth2",
			setup: func(in *apiv1.ApiExposure) {
				in.Spec.Security = &apiv1.Security{M2M: &apiv1.Machine2MachineAuthentication{ExternalIDP: &apiv1.ExternalIdentityProvider{TokenEndpoint: "https://idp/token", TokenRequest: "body", GrantType: "client_credentials", Client: &apiv1.OAuth2ClientCredentials{ClientId: "cid", ClientSecret: "sec"}}, Scopes: []string{"s1"}}}
			},
			assert: func(t *testing.T, out api.ApiExposureResponse) {
				t.Helper()
				got, err := out.Security.AsOAuth2()
				if err != nil {
					t.Fatalf("expected oauth2 security, got err: %v", err)
				}
				if got.TokenEndpoint != "https://idp/token" || got.ClientId != "cid" || got.ClientSecret != "sec" || len(got.Scopes) != 1 {
					t.Fatalf("unexpected oauth2 mapping: %#v", got)
				}
			},
		},
		{
			name: "external idp oauth2 with basic credentials",
			setup: func(in *apiv1.ApiExposure) {
				in.Spec.Security = &apiv1.Security{M2M: &apiv1.Machine2MachineAuthentication{ExternalIDP: &apiv1.ExternalIdentityProvider{TokenEndpoint: "https://idp/token", TokenRequest: "header", GrantType: "password", Basic: &apiv1.BasicAuthCredentials{Username: "bu", Password: "bp"}}}}
			},
			assert: func(t *testing.T, out api.ApiExposureResponse) {
				t.Helper()
				got, err := out.Security.AsOAuth2()
				if err != nil {
					t.Fatalf("expected oauth2 security, got err: %v", err)
				}
				if got.Username != "bu" || got.Password != "bp" {
					t.Fatalf("unexpected oauth2 basic mapping: %#v", got)
				}
			},
		},
		{
			name: "scopes only oauth2",
			setup: func(in *apiv1.ApiExposure) {
				in.Spec.Security = &apiv1.Security{M2M: &apiv1.Machine2MachineAuthentication{Scopes: []string{"scope-a", "scope-b"}}}
			},
			assert: func(t *testing.T, out api.ApiExposureResponse) {
				t.Helper()
				got, err := out.Security.AsOAuth2()
				if err != nil {
					t.Fatalf("expected oauth2 security, got err: %v", err)
				}
				if len(got.Scopes) != 2 {
					t.Fatalf("unexpected scopes mapping: %#v", got.Scopes)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &apiv1.ApiExposure{}
			out := api.ApiExposureResponse{}
			tt.setup(in)
			mapSecurity(in, &out)
			tt.assert(t, out)
		})
	}
}

func TestVisibilityAndApprovalFallback(t *testing.T) {
	t.Parallel()

	in := &apiv1.ApiExposure{}
	out := api.ApiExposureResponse{}
	mapTransformation(in, &out)
	if len(out.RemoveHeaders) != 0 {
		t.Fatalf("expected no removed headers for nil transformation, got %#v", out.RemoveHeaders)
	}

	if got := toAPIVisibility(apiv1.VisibilityZone); got != api.ZONE {
		t.Fatalf("unexpected zone visibility mapping: %q", got)
	}
	if got := toAPIVisibility(apiv1.VisibilityEnterprise); got != api.ENTERPRISE {
		t.Fatalf("unexpected enterprise visibility mapping: %q", got)
	}

	if got := toAPIVisibility(apiv1.Visibility("custom")); got != api.Visibility("CUSTOM") {
		t.Fatalf("unexpected visibility fallback: %q", got)
	}
	if got := toAPIApprovalStrategy(apiv1.ApprovalStrategyAuto); got != api.AUTO {
		t.Fatalf("unexpected auto approval mapping: %q", got)
	}
	if got := toAPIApprovalStrategy(apiv1.ApprovalStrategyFourEyes); got != api.FOUREYES {
		t.Fatalf("unexpected four eyes approval mapping: %q", got)
	}
	if got := toAPIApprovalStrategy(apiv1.ApprovalStrategy("custom")); got != api.ApprovalStrategy("CUSTOM") {
		t.Fatalf("unexpected approval fallback: %q", got)
	}
}
