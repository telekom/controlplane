// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ features.Feature = &FailoverFeature{}

// FailoverFeature implements the failover feature for routes.
type FailoverFeature struct {
	priority int
}

var InstanceFailoverFeature = &FailoverFeature{
	priority: InstanceCircuitBreakerFeature.priority - 1,
}

func (f *FailoverFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeFailover
}

func (f *FailoverFeature) Priority() int {
	return f.priority
}

func (f *FailoverFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return route.Spec.Traffic.Failover != nil && len(route.Spec.Traffic.Failover.Targets) > 0
}

func (f *FailoverFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	routingConfigs := builder.RoutingConfigs()
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	failover := route.Spec.Traffic.Failover
	envName := contextutil.EnvFromContextOrDie(ctx)

	builder.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))

	// This is the proxy upstream that should be used in all non-failover cases (primary upstream).
	// The target is the zone where the API is exposed on.
	upstream := route.Spec.Backend.Upstreams[0]
	proxyRoutingCfg := &plugin.RoutingConfig{
		RemoteApiUrl:   upstream.Url(),
		ApiBasePath:    upstream.Path,
		Realm:          route.Spec.Security.RealmName,
		Environment:    envName,
		TargetZoneName: failover.TargetZoneName, // The zone where the API is exposed on
		JumperConfig:   nil,                     // JumperConfig is not needed for the proxy routing config
	}
	routingConfigs.Add(proxyRoutingCfg)

	// The failover targets are used if the Jumper has determined that the primary upstream is not available.
	// It does so by checking the health of the TargetZoneName using the ZoneHealthCheckService.

	hasFailoverSecurity := HasFailoverSecurity(route)
	isFailoverSecondary := route.Spec.Type == gatewayv1.RouteTypeSecondary
	failoverRealm := route.Spec.Traffic.Failover.Security.RealmName
	if failoverRealm == "" {
		failoverRealm = route.Spec.Security.RealmName
	}

	if isFailoverSecondary {
		// This route is a failover secondary route. Its failover targets are the real backend upstreams.
		jumperCfg := builder.JumperConfig()
		routingCfg := &plugin.RoutingConfig{
			JumperConfig: jumperCfg,
			Realm:        failoverRealm,
			Environment:  envName,
		}
		routingConfigs.Add(routingCfg)

		hasLoadbalancing := len(failover.Targets) > 1
		if hasLoadbalancing {
			routingCfg.LoadBalancing = mapFailoverTargetsToLoadBalancingServers(failover.Targets)
		} else {
			routingCfg.RemoteApiUrl = failover.Targets[0].Upstream.Url()
			routingCfg.ApiBasePath = failover.Targets[0].Upstream.Path
		}

		// Because (per default) the ExternalIDP feature will set this header, we need to remove it here
		// to avoid conflicts with the failover security configuration.
		if hasFailoverSecurity && route.Spec.Traffic.Failover.Security.HasM2MExternalIDP() {
			routingCfg.TokenEndpoint = failover.Security.M2M.ExternalIDP.TokenEndpoint
			builder.RequestTransformerPlugin().Config.Append.Headers.Remove("token_endpoint")
		}

	} else {
		// This is a proxy route. Each failover target is a secondary zone's gateway.
		// The jumper iterates the list and picks the first one whose zone is healthy.
		jumperCfg := builder.JumperConfig()
		// In failover-scenario, this is also a proxy-route so we need to set mesh=true to allow the jumper to route to other zones.
		jumperCfg.Mesh = true
		for _, target := range failover.Targets {
			routingCfg := &plugin.RoutingConfig{
				JumperConfig:   jumperCfg,
				Realm:          failoverRealm,
				Environment:    envName,
				RemoteApiUrl:   target.Upstream.Url(),
				ApiBasePath:    target.Upstream.Path,
				TargetZoneName: target.ZoneName,
			}
			routingConfigs.Add(routingCfg)
		}
	}

	return nil
}
