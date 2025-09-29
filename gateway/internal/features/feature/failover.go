// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	"github.com/pkg/errors"
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
	return route.Spec.Traffic.Failover != nil && len(route.Spec.Traffic.Failover.Upstreams) > 0
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
	upstream := route.Spec.Upstreams[0]
	proxyRoutingCfg := &plugin.RoutingConfig{
		RemoteApiUrl:   upstream.Url(),
		ApiBasePath:    upstream.Path,
		Realm:          route.Spec.Realm.Name,
		Issuer:         upstream.IssuerUrl,
		ClientId:       upstream.ClientId,
		ClientSecret:   upstream.ClientSecret,
		Environment:    envName,
		TargetZoneName: failover.TargetZoneName, // The zone where the API is exposed on
		JumperConfig:   nil,                     // JumperConfig ist not needed for the proxy routing config
	}
	routingConfigs.Add(proxyRoutingCfg)

	// The failover upstreams are used if the Jumper has determined that the primary upstream is not available.
	// It does so by checking the health of the TargetZoneName using the ZoneHealthCheckService.

	hasLoadbalancing := len(failover.Upstreams) > 1
	hasFailoverSecurity := route.HasFailoverSecurity()
	IsFailoverSecondary := route.IsFailoverSecondary()

	jumperCfg := builder.JumperConfig()
	routingCfg := &plugin.RoutingConfig{
		JumperConfig: jumperCfg,
		Realm:        route.Spec.Realm.Name,
		Environment:  envName,
	}
	routingConfigs.Add(routingCfg)

	if IsFailoverSecondary {
		// This route is a failover secondary route. This means that its failover-upstream is the real primary upstream.
		if hasLoadbalancing {
			// In addition, the real primary upstream is a loadbalancing upstream.
			routingCfg.LoadBalancing = mapToLoadBalancingServers(failover.Upstreams)

		} else {
			// The real primary upstream is a single failover upstream.
			routingCfg.RemoteApiUrl = failover.Upstreams[0].Url()
			routingCfg.ApiBasePath = failover.Upstreams[0].Path
		}

		// Because (per default) the ExternalIDP feature will set this header, we need to remove it here
		// to avoid conflicts with the failover security configuration.
		if hasFailoverSecurity && route.Spec.Traffic.Failover.Security.HasM2MExternalIDP() {
			routingCfg.TokenEndpoint = failover.Security.M2M.ExternalIDP.TokenEndpoint
			builder.RequestTransformerPlugin().Config.Append.Headers.Remove("token_endpoint")
		}

	} else {
		if hasLoadbalancing {
			return errors.New("loadbalancing is not supported for proxy routes that are not failover secondary routes")
		}

		// This route is not a failover secondary route. It is a proxy route that proxies to the secondary failover upstream.
		failoverUpstream := failover.Upstreams[0]
		routingCfg.RemoteApiUrl = failoverUpstream.Url()
		routingCfg.ApiBasePath = failoverUpstream.Path
		routingCfg.Issuer = failoverUpstream.IssuerUrl
		routingCfg.ClientId = failoverUpstream.ClientId
		routingCfg.ClientSecret = failoverUpstream.ClientSecret
		routingCfg.TargetZoneName = "" // No target zone for failover upstreams
	}

	return nil
}
