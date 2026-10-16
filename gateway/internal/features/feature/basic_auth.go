// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	"github.com/pkg/errors"

	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	secretManagerApi "github.com/telekom/controlplane/secret-manager/api"
)

var _ features.Feature = (*BasicAuthFeature)(nil)

type BasicAuthFeature struct {
	priority int
}

var InstanceBasicAuthFeature = &BasicAuthFeature{
	priority: 10,
}

func (b *BasicAuthFeature) Name() v1.FeatureType {
	return v1.FeatureTypeBasicAuth
}

func (b *BasicAuthFeature) Priority() int {
	return b.priority
}

func (b *BasicAuthFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	// Check if route exists
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}

	// Skip if passthrough is enabled
	if route.Spec.PassThrough {
		return false
	}

	// Check for failover security with basic auth
	if HasFailoverSecurity(route) && route.Spec.Traffic.Failover.Security.HasBasicAuth() {
		return true
	}

	// For primary routes, check route security and all consumers
	if !route.IsProxy() {
		// Check if route itself has basic auth configured
		if HasM2M(route) && route.Spec.Security.HasBasicAuth() {
			return true
		}

		// Check if any consumer has basic auth configured
		for _, consumer := range builder.GetAllowedConsumers() {
			if consumer.HasM2MBasic() && !usesPasswordGrantCredentials(route, consumer) {
				return true
			}
		}
	}

	return false
}

func (b *BasicAuthFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	jumperConfig := builder.JumperConfig()
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	security := route.Spec.Security
	if HasFailoverSecurity(route) {
		security = route.Spec.Traffic.Failover.Security
	}

	if security.HasBasicAuth() {
		passwordValue, err := secretManagerApi.Get(ctx, security.M2M.Basic.Password)
		if err != nil {
			return errors.Wrapf(err, "cannot get basic auth password for route %s", route.GetName())
		}
		jumperConfig.BasicAuth[DefaultProviderKey] = plugin.BasicAuthCredentials{
			Username: security.M2M.Basic.Username,
			Password: passwordValue,
		}
	}

	for _, consumer := range builder.GetAllowedConsumers() {
		if !consumer.HasM2MBasic() || usesPasswordGrantCredentials(route, consumer) {
			continue
		}
		security := consumer.Spec.Security

		passwordValue, err := secretManagerApi.Get(ctx, security.M2M.Basic.Password)
		if err != nil {
			return errors.Wrapf(err, "cannot get basic auth password for consumer %s", consumer.Spec.ConsumerName)
		}
		jumperConfig.BasicAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)] = plugin.BasicAuthCredentials{
			Username: security.M2M.Basic.Username,
			Password: passwordValue,
		}
	}

	return nil
}

func usesPasswordGrantCredentials(route *v1.Route, consumer *v1.ConsumeRoute) bool {
	if !consumer.HasM2M() || !consumer.HasM2MBasic() {
		return false
	}
	credentials := consumer.Spec.Security.M2M
	if len(credentials.Scopes) == 0 && credentials.Client == nil {
		return false
	}

	security := route.Spec.Security
	if route.IsFailoverSecondary() && route.Spec.Traffic.Failover != nil {
		security = route.Spec.Traffic.Failover.Security
	}
	return security.HasM2MExternalIDP() && security.M2M.ExternalIDP.GrantType == v1.GrantTypePassword &&
		(route.IsPrimary() || route.IsFailoverSecondary())
}
