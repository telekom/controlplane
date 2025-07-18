// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ features.Feature = &LoadBalancingFeature{}

type LoadBalancingFeature struct {
	priority int
}

var InstanceLoadBalancingFeature = &LoadBalancingFeature{
	priority: InstanceLastMileSecurityFeature.priority + 2,
}

func (f *LoadBalancingFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeLoadBalancing
}

func (f *LoadBalancingFeature) Priority() int {
	return f.priority
}

func (f *LoadBalancingFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return len(route.Spec.Upstreams) > 1
}

func (f *LoadBalancingFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}
	upstreams := route.Spec.Upstreams

	// Upstream URL is always set to jumper proxy URL (localhost for sidecar proxy)
	builder.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))

	// Load Balancing is added to JumperConfig
	jumperConfig := builder.JumperConfig()
	jumperConfig.LoadBalancing = mapToLoadBalancingServers(upstreams)

	// Check if the remote API URL header should be removed when using Last Mile Security
	RemoveRemoteApiUrlHeaderIfNeeded(route, builder.RequestTransformerPlugin())

	return nil
}

func mapToLoadBalancingServers(upstreams []gatewayv1.Upstream) *plugin.LoadBalancing {
	servers := make([]plugin.LoadBalancingServer, 0, len(upstreams))
	for _, upstream := range upstreams {
		servers = append(servers, mapToLoadBalancingServer(upstream))
	}
	return &plugin.LoadBalancing{
		Servers: servers,
	}
}

func mapToLoadBalancingServer(upstream gatewayv1.Upstream) plugin.LoadBalancingServer {
	return plugin.LoadBalancingServer{
		Upstream: upstream.Url(),
		Weight:   upstream.Weight,
	}
}

func RemoveRemoteApiUrlHeaderIfNeeded(route *gatewayv1.Route, rtp *plugin.RequestTransformerPlugin) {
	lastMileSecurityIsUsed := !route.Spec.PassThrough
	realRoute := !route.IsProxy()

	if realRoute && lastMileSecurityIsUsed {

		// Remove the remote_api_url header if it exists:
		// This is necessary to avoid conflicts with Last Mile Security in Jumper, because Jumper will
		// only consider LoadBalancing servers if the remote_api_url header is not set.
		if rtp.Config.Append.Headers != nil && rtp.Config.Append.Headers.Contains("remote_api_url") {
			rtp.Config.Append.Headers.Remove("remote_api_url")
		}
	}
}
