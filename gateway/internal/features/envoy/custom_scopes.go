// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"strings"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ EnvoyFeature = &CustomScopesFeature{}

// CustomScopesFeature is the Envoy counterpart of feature.CustomScopesFeature
// (internal/features/feature/custom_scopes.go). The Kong path writes per-consumer
// OAuth2 scopes into JumperConfig.OAuth[ConsumerId].Scopes, base64-encoded into a
// header the Jumper sidecar reads when it mints the last-mile (LMS) token.
//
// In Envoy the LMS token is minted by the external gRPC ext_authz issuer. This
// feature declares the resolved scope map as intent; the builder folds it into
// the ext_authz per-route context_extensions (see customScopesIntent). The
// issuer selects the scope set by matching the incoming clientId/azp claim it
// receives via metadata_context from jwt_authn.
type CustomScopesFeature struct {
	priority int
}

// InstanceCustomScopesFeature mirrors the Kong
// feature.InstanceCustomScopesFeature priority (10,
// internal/features/feature/custom_scopes.go:22-24).
var InstanceCustomScopesFeature = &CustomScopesFeature{priority: 10}

func (f *CustomScopesFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeCustomScopes
}

func (f *CustomScopesFeature) Priority() int { return f.priority }

// IsUsed mirrors feature.CustomScopesFeature.IsUsed
// (internal/features/feature/custom_scopes.go:34-44): used for a primary or
// failover-secondary route that is not pass-through.
func (f *CustomScopesFeature) IsUsed(ctx context.Context, builder features.FeatureBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	notPassThrough := !route.Spec.PassThrough
	isPrimaryRoute := !route.IsProxy()
	isFailoverSecondary := route.Spec.Type == gatewayv1.RouteTypeSecondary

	return notPassThrough && (isPrimaryRoute || isFailoverSecondary)
}

// Apply reads the same source fields as the Kong path's Apply
// (internal/features/feature/custom_scopes.go:46-81):
//   - route.Spec.Security.M2M.Scopes -> the default bucket (applies to any
//     consumer without an explicit scope entry), and
//   - each allowed ConsumeRoute's Spec.Security.M2M.Scopes -> keyed by
//     ConsumerName.
//
// Scopes are joined with a space, matching the Kong path
// (custom_scopes.go:62,72). The resolved map is declared as intent; the builder
// emits it as ext_authz context_extensions.
func (f *CustomScopesFeature) Apply(ctx context.Context, builder EnvoyFeatureBuilder) error {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	// ponytail: the Kong path short-circuits when OAuth is already populated by
	// the external-IDP feature (feature/custom_scopes.go:53-56 "already
	// populated by external_idp feature"). The Envoy ExternalIDP counterpart
	// does not exist yet; when it lands it must run first (lower priority) and
	// this Apply must yield to it. Upgrade path: add an externalIDP intent bit
	// and return early here when set.

	defaultScopes := ""
	if route.Spec.Security.M2M != nil && len(route.Spec.Security.M2M.Scopes) > 0 {
		defaultScopes = strings.Join(route.Spec.Security.M2M.Scopes, " ")
	}

	perConsumer := map[string]string{}
	for _, cr := range builder.GetAllowedConsumers() {
		if cr.Spec.Security != nil && cr.Spec.Security.M2M != nil && len(cr.Spec.Security.M2M.Scopes) > 0 {
			perConsumer[cr.Spec.ConsumerName] = strings.Join(cr.Spec.Security.M2M.Scopes, " ")
		}
	}

	if defaultScopes == "" && len(perConsumer) == 0 {
		return nil
	}

	builder.AddCustomScopes(defaultScopes, perConsumer)
	return nil
}
