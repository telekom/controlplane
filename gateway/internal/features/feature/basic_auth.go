// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	v1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
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
	route := builder.GetRoute()

	notPassThrough := !route.Spec.PassThrough
	isPrimaryRoute := !route.IsProxy()
	isConfigured := false

	if route.HasFailoverSecurity() {
		isConfigured = route.Spec.Traffic.Failover.Security.HasBasicAuth()
	}

	if isPrimaryRoute && route.HasM2M() {
		isConfigured = route.Spec.Security.HasBasicAuth()
	}

	return notPassThrough && isConfigured
}

func (b *BasicAuthFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	jumperConfig := builder.JumperConfig()
	route := builder.GetRoute()

	security := route.Spec.Security
	if route.HasFailoverSecurity() {
		security = route.Spec.Traffic.Failover.Security
	}

	jumperConfig.BasicAuth[plugin.ConsumerId(DefaultProviderKey)] = plugin.BasicAuthCredentials{
		Username: security.M2M.Basic.Username,
		Password: security.M2M.Basic.Password,
	}

	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.Spec.Security == nil {
			continue
		}
		security := consumer.Spec.Security

		if !security.HasBasicAuth() {
			continue
		}
		jumperConfig.BasicAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)] = plugin.BasicAuthCredentials{
			Username: security.M2M.Basic.Username,
			Password: security.M2M.Basic.Password,
		}
	}

	return nil
}
