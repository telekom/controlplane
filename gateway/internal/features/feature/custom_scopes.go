// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"strings"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ features.Feature = &CustomScopesFeature{}

type CustomScopesFeature struct {
	priority int
}

var InstanceCustomScopesFeature = &CustomScopesFeature{
	priority: 10,
}

func (f *CustomScopesFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeCustomScopes
}

func (f *CustomScopesFeature) Priority() int {
	return f.priority
}

func (f *CustomScopesFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	notPassThrough := !route.Spec.PassThrough
	isPrimaryRoute := !route.IsProxy()
	isFailoverSecondary := route.IsFailoverSecondary()

	return notPassThrough && (isPrimaryRoute || isFailoverSecondary)
}

func (f *CustomScopesFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	jumperConfig := builder.JumperConfig()
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	if len(jumperConfig.OAuth) > 0 {
		// already populated by external_idp feature
		return nil
	}

	// Default scopes
	// If the route has a security configuration with default M2M scopes, we add them to the JumperConfig
	if route.Spec.Security != nil && route.Spec.Security.M2M != nil {
		if len(route.Spec.Security.M2M.Scopes) > 0 {
			// Join scopes with a space, as Kong expects a single string with space-separated scopes
			jumperConfig.OAuth[DefaultProviderKey] = plugin.OauthCredentials{
				Scopes: strings.Join(route.Spec.Security.M2M.Scopes, " "),
			}
		}
	}

	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.Spec.Security != nil && consumer.Spec.Security.M2M != nil {
			if len(consumer.Spec.Security.M2M.Scopes) > 0 {
				// Join scopes with a space, as Kong expects a single string with space-separated scopes
				jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)] = plugin.OauthCredentials{
					Scopes: strings.Join(consumer.Spec.Security.M2M.Scopes, " "),
				}
			}
		}

	}

	return nil
}
