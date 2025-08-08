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
	if route.HasFailoverSecurity() && route.Spec.Traffic.Failover.Security.HasBasicAuth() {
		return true
	}

	// For primary routes, check route security and all consumers
	if !route.IsProxy() {
		// Check if route itself has basic auth configured
		if route.HasM2M() && route.Spec.Security.HasBasicAuth() {
			return true
		}

		// Check if any consumer has basic auth configured
		for _, consumer := range builder.GetAllowedConsumers() {
			if consumer.HasM2M() && consumer.Spec.Security.HasBasicAuth() {
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
	if route.HasFailoverSecurity() {
		security = route.Spec.Traffic.Failover.Security
	}

	if security != nil && security.HasBasicAuth() {
		passwordValue, err := secretManagerApi.Get(ctx, security.M2M.Basic.Password)
		if err != nil {
			return errors.Wrapf(err, "cannot get basic auth password for route %s", route.GetName())
		}
		jumperConfig.BasicAuth[plugin.ConsumerId(DefaultProviderKey)] = plugin.BasicAuthCredentials{
			Username: security.M2M.Basic.Username,
			Password: passwordValue,
		}
	}

	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.Spec.Security == nil {
			continue
		}
		security := consumer.Spec.Security

		if !security.HasBasicAuth() {
			continue
		}
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
