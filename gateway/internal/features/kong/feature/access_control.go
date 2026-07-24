// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ features.KongFeature = &AccessControlFeature{}

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

func (f *AccessControlFeature) IsUsed(ctx context.Context, builder features.KongFeatureBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return len(route.GetTrustedIssuers()) > 0
}

func (f *AccessControlFeature) Apply(ctx context.Context, builder features.KongFeatureBuilder) (err error) {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}
	hasIssuer := len(route.GetTrustedIssuers()) > 0
	if hasIssuer {
		// This will initialize the JWT-Plugin and set the issuer URLs of the downstreams
		builder.JwtPlugin()
	}

	// If access control is disabled, we skip the ACL plugin setup
	if route.Spec.Security.DisableAccessControl {
		return nil
	}

	aclPlugin := builder.AclPlugin()

	for _, defaultConsumer := range route.Spec.Security.DefaultConsumers {
		aclPlugin.Config.Allow.Add(defaultConsumer)
	}

	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.Spec.Route.Equals(route) {
			// Only add allowed consumers that actually belong to this specific route
			aclPlugin.Config.Allow.Add(consumer.Spec.ConsumerName)
		}
	}

	// If no consumers were added, use a sentinel group to deny all traffic.
	// Kong requires the ACL allow list to be non-empty; this placeholder ensures
	// the plugin is accepted while no real consumer can match it.
	if aclPlugin.Config.Allow.Empty() {
		aclPlugin.Config.Allow.Add(plugin.DenyAllGroup)
	}

	return nil
}
