// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"fmt"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	jwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	rbachttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/go-logr/logr"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

// Canonical Envoy filter names. jwtAuthnFilterName is also the dynamic-metadata
// namespace under which jwt_authn publishes the verified payload, which rbac
// reads back.
const (
	jwtAuthnFilterName = "envoy.filters.http.jwt_authn"
	rbacFilterName     = "envoy.filters.http.rbac"
)

// payloadInMetadataKey is the shared second-level metadata key every provider
// writes its payload under, so the rbac principal can match azp on the path
// [payloadInMetadataKey, "azp"] regardless of which issuer verified the token.
const payloadInMetadataKey = "jwt_payload"

// consumerMatchClaim mirrors the Kong path (plugin.ConsumerMatchClaim = "azp"):
// the JWT claim whose value identifies the calling consumer.
const consumerMatchClaim = "azp"

var _ features.EnvoyFeature = &AccessControlFeature{}

type AccessControlFeature struct {
	priority int
}

// InstanceAccessControlFeature is the registered AccessControl feature. Priority
// mirrors the Kong AccessControlFeature (10).
var InstanceAccessControlFeature = &AccessControlFeature{
	priority: 10,
}

// Name implements [features.Feature].
func (*AccessControlFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeAccessControl
}

// Priority implements [features.Feature].
func (f *AccessControlFeature) Priority() int {
	return f.priority
}

// IsUsed implements [features.Feature]. AccessControl applies when the route
// declares trusted issuers (mirrors kong/feature/access_control.go:38).
func (f *AccessControlFeature) IsUsed(_ context.Context, builder features.EnvoyFeatureBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return len(route.GetTrustedIssuers()) > 0
}

// Apply implements [features.Feature]. It contributes:
//   - a jwt_authn filter that verifies the token and requires that its issuer is
//     one of the route's trusted issuers (rejects otherwise), publishing the
//     payload to dynamic metadata; and
//   - unless DisableAccessControl, an rbac filter that allows the request only if
//     the token's azp claim is in the consumer allow-list (default consumers plus
//     the route's allowed consumers). An empty allow-list denies all traffic.
func (f *AccessControlFeature) Apply(ctx context.Context, builder features.EnvoyFeatureBuilder) error {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.feature.access-control")
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	issuers := route.GetTrustedIssuers()
	jwtCfg, err := buildJwtAuthn(issuers)
	if err != nil {
		return fmt.Errorf("building jwt_authn config: %w", err)
	}
	builder.AddHTTPFilter(jwtAuthnFilterName, jwtCfg)
	log.V(1).Info("Added jwt_authn filter", "issuers", len(issuers))

	if route.Spec.Security.DisableAccessControl {
		log.V(1).Info("AccessControl disabled, skipping rbac filter")
		return nil
	}

	allow := consumerAllowList(route, builder.GetAllowedConsumers())
	rbacCfg, err := buildRBAC(allow)
	if err != nil {
		return fmt.Errorf("building rbac config: %w", err)
	}
	builder.AddHTTPFilter(rbacFilterName, rbacCfg)
	log.V(0).Info("Configured access control", "allowedConsumers", len(allow))
	return nil
}

// consumerAllowList collects the consumer names permitted on the route: the
// configured default consumers plus every allowed consumer that belongs to this
// specific route (mirrors kong/feature/access_control.go:59-68). Order-stable,
// deduplicated.
func consumerAllowList(route *gatewayv1.Route, allowed []*gatewayv1.ConsumeRoute) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(name string) {
		if _, dup := seen[name]; dup {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}

	for _, dc := range route.Spec.Security.DefaultConsumers {
		add(dc)
	}
	for _, consumer := range allowed {
		if consumer.Spec.Route.Equals(route) {
			add(consumer.Spec.ConsumerName)
		}
	}
	return out
}

// buildJwtAuthn builds a JwtAuthentication that accepts a token from any of the
// trusted issuers (requires_any over one provider per issuer) and rejects
// missing/invalid tokens. Every provider publishes its payload under the shared
// payloadInMetadataKey so rbac can match azp uniformly.
//
// ponytail: remote JWKS URI is derived as <issuer>/protocol/openid-connect/certs
// (Keycloak convention) and fetched via a per-issuer cluster named
// "<issuer>-jwks". Those clusters are not emitted yet — wire them into the
// bundle (or switch to a shared discovery cluster) before this serves traffic.
func buildJwtAuthn(issuers []string) (*anypb.Any, error) {
	providers := make(map[string]*jwtauthnv3.JwtProvider, len(issuers))
	anyReqs := make([]*jwtauthnv3.JwtRequirement, 0, len(issuers))

	for i, issuer := range issuers {
		name := fmt.Sprintf("provider-%d", i)
		providers[name] = &jwtauthnv3.JwtProvider{
			Issuer:            issuer,
			Forward:           true,
			PayloadInMetadata: payloadInMetadataKey,
			JwksSourceSpecifier: &jwtauthnv3.JwtProvider_RemoteJwks{
				RemoteJwks: &jwtauthnv3.RemoteJwks{
					HttpUri: &corev3.HttpUri{
						Uri:              issuer + "/protocol/openid-connect/certs",
						Timeout:          durationpb.New(5 * time.Second),
						HttpUpstreamType: &corev3.HttpUri_Cluster{Cluster: name + "-jwks"},
					},
					CacheDuration: durationpb.New(5 * time.Minute),
				},
			},
		}
		anyReqs = append(anyReqs, &jwtauthnv3.JwtRequirement{
			RequiresType: &jwtauthnv3.JwtRequirement_ProviderName{ProviderName: name},
		})
	}

	cfg := &jwtauthnv3.JwtAuthentication{
		Providers: providers,
		Rules: []*jwtauthnv3.RequirementRule{{
			Match: &routev3.RouteMatch{
				PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/"},
			},
			RequirementType: &jwtauthnv3.RequirementRule_Requires{
				Requires: &jwtauthnv3.JwtRequirement{
					RequiresType: &jwtauthnv3.JwtRequirement_RequiresAny{
						RequiresAny: &jwtauthnv3.JwtRequirementOrList{Requirements: anyReqs},
					},
				},
			},
		}},
	}
	return anypb.New(cfg)
}

// buildRBAC builds an ALLOW rbac filter whose single policy permits any request
// whose azp claim (published by jwt_authn under payloadInMetadataKey) matches one
// of the allowed consumer names. An empty allow-list yields an ALLOW rbac with no
// policies, which denies all requests (mirrors Kong's DenyAllGroup sentinel).
func buildRBAC(allowedConsumers []string) (*anypb.Any, error) {
	rules := &rbacconfigv3.RBAC{
		Action:   rbacconfigv3.RBAC_ALLOW,
		Policies: map[string]*rbacconfigv3.Policy{},
	}

	if len(allowedConsumers) > 0 {
		principals := make([]*rbacconfigv3.Principal, 0, len(allowedConsumers))
		for _, name := range allowedConsumers {
			principals = append(principals, &rbacconfigv3.Principal{
				Identifier: &rbacconfigv3.Principal_Metadata{
					Metadata: &matcherv3.MetadataMatcher{
						Filter: jwtAuthnFilterName,
						Path: []*matcherv3.MetadataMatcher_PathSegment{
							{Segment: &matcherv3.MetadataMatcher_PathSegment_Key{Key: payloadInMetadataKey}},
							{Segment: &matcherv3.MetadataMatcher_PathSegment_Key{Key: consumerMatchClaim}},
						},
						Value: &matcherv3.ValueMatcher{
							MatchPattern: &matcherv3.ValueMatcher_StringMatch{
								StringMatch: &matcherv3.StringMatcher{
									MatchPattern: &matcherv3.StringMatcher_Exact{Exact: name},
								},
							},
						},
					},
				},
			})
		}
		rules.Policies["allow-consumers"] = &rbacconfigv3.Policy{
			Permissions: []*rbacconfigv3.Permission{
				{Rule: &rbacconfigv3.Permission_Any{Any: true}},
			},
			Principals: principals,
		}
	}

	return anypb.New(&rbachttpv3.RBAC{Rules: rules})
}
