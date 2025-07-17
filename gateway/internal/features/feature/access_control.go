// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"slices"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ features.Feature = &AccessControlFeature{}

type AccessControlFeature struct {
	priority int
}

var InstanceAccessControlFeature = &AccessControlFeature{
	priority: 10,
}

func (f *AccessControlFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeAccessControl
}

func (f *AccessControlFeature) Priority() int {
	return f.priority
}

func (f *AccessControlFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route := builder.GetRoute()
	hasIssuerDefined := len(route.Spec.Downstreams) > 0 && route.Spec.Downstreams[0].IssuerUrl != ""
	return hasIssuerDefined
}

func (f *AccessControlFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	route := builder.GetRoute()
	hasIssuer := slices.ContainsFunc(route.Spec.Downstreams, func(downstream gatewayv1.Downstream) bool {
		return downstream.IssuerUrl != ""
	})
	if hasIssuer {
		// This will initialize the JWT-Plugin and set the issuer URLs of the downstreams
		builder.JwtPlugin()
	}

	if route.Spec.Security != nil && !route.Spec.Security.DisableAccessControl {
		aclPlugin := builder.AclPlugin()

		aclPlugin.Config.Allow.Add("gateway")
		for _, defaultConsumer := range builder.GetRealm().Spec.DefaultConsumers {
			aclPlugin.Config.Allow.Add(defaultConsumer)
		}

		for _, consumer := range builder.GetAllowedConsumers() {
			if consumer.Spec.Route.Equals(route) {
				// Only add allowed consumers that actually belong to this specific route
				aclPlugin.Config.Allow.Add(consumer.Spec.ConsumerName)
			}
		}
	}

	return nil
}
