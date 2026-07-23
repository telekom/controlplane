// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ KGatewayFeature = &AccessControlFeature{}

// AccessControlFeature is the kgateway counterpart of
// feature.AccessControlFeature and envoy.AccessControlFeature. It reads the same
// Route source fields and declares the equivalent intent (require JWT for
// trusted issuers; allow a consumer set, empty => deny-all) onto the kgateway
// builder, which renders it into a TrafficPolicy + GatewayExtension.
type AccessControlFeature struct {
	priority int
}

// InstanceAccessControlFeature is the shared instance, mirroring the Kong and
// Envoy feature instances (priority 10).
var InstanceAccessControlFeature = &AccessControlFeature{priority: 10}

func (f *AccessControlFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeAccessControl
}

func (f *AccessControlFeature) Priority() int { return f.priority }

// IsUsed mirrors envoy.AccessControlFeature.IsUsed
// (internal/features/envoy/access_control.go:58-64): used when the route has
// trusted issuers.
func (f *AccessControlFeature) IsUsed(ctx context.Context, builder features.FeatureBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return len(route.GetTrustedIssuers()) > 0
}

// Apply mirrors envoy.AccessControlFeature.Apply
// (internal/features/envoy/access_control.go:71-106): require JWT for the
// route's trusted issuers, and (unless access control is disabled) build the
// consumer allow-list from default consumers plus route-matched allowed
// consumers. An empty allow-list means deny-all.
func (f *AccessControlFeature) Apply(ctx context.Context, builder KGatewayFeatureBuilder) error {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	if len(route.GetTrustedIssuers()) > 0 {
		builder.RequireJWT(route.GetTrustedIssuers())
	}

	if route.Spec.Security.DisableAccessControl {
		return nil
	}

	seen := map[string]struct{}{}
	allowed := make([]string, 0)
	add := func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		allowed = append(allowed, name)
	}

	for _, dc := range route.Spec.Security.DefaultConsumers {
		add(dc)
	}
	for _, cr := range builder.GetAllowedConsumers() {
		if cr.Spec.Route.Equals(route) {
			add(cr.Spec.ConsumerName)
		}
	}

	builder.AllowConsumers(allowed)
	return nil
}
