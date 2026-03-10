// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"fmt"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ features.Feature = &DynamicUpstreamFeature{}

var InstanceDynamicUpstreamFeature = &DynamicUpstreamFeature{
	// Priority is set to be higher than InstanceLastMileSecurityFeature to ensure
	// that the value of "remote_api_url" is set by this feature
	priority: InstanceLastMileSecurityFeature.Priority() + 1,
}

type DynamicUpstreamFeature struct {
	priority int
}

// Name implements features.Feature.
func (d *DynamicUpstreamFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeDynamicUpstream
}

// Priority implements features.Feature.
func (d *DynamicUpstreamFeature) Priority() int {
	return d.priority
}

// IsUsed implements features.Feature.
func (d *DynamicUpstreamFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}

	if len(route.Spec.Upstreams) != 1 {
		// Must have exactly 1 upstream to be considered for dynamic upstream feature
		return false
	}
	if route.Spec.Upstreams[0].IsProxy() {
		// Dynamic Upstream is only relevant for non-proxy routes (last-hop)
		return false
	}
	if route.Spec.Upstreams[0].Host != "localhost" {
		// Dynamic Upstream is only relevant if the upstream host is set to "localhost"
		// (indicating that it should be replaced with the actual target URL at runtime)
		return false
	}

	return route.HasDynamicUpstream()

}

// Apply implements features.Feature.
func (d *DynamicUpstreamFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}
	urlParameter := route.Spec.Traffic.DynamicUpstream.QueryParameter

	rtpPlugin := builder.RequestTransformerPlugin()

	// Override the static remote_api_url (set by LastMileSecurity) with the dynamic value
	rtpPlugin.Config.Append.Headers.Remove("remote_api_url")
	rtpPlugin.Config.Append.
		AddHeader("remote_api_url", fmt.Sprintf("$(query_params.%s)", urlParameter))
	// Remove the URL parameter from the forwarded request
	rtpPlugin.Config.Remove.AddQuerystring(urlParameter)

	return nil
}
