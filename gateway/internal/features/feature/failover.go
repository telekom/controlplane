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
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ features.Feature = &FailoverFeature{}

// FailoverFeature implements the failover feature for routes.
type FailoverFeature struct {
	priority int
}

var InstanceFailoverFeature = &FailoverFeature{
	priority: InstanceLastMileSecurityFeature.priority - 10,
}

func (f *FailoverFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeFailover
}

func (f *FailoverFeature) Priority() int {
	return f.priority
}

func (f *FailoverFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route := builder.GetRoute()
	return route.Spec.Traffic.Failover != nil && len(route.Spec.Traffic.Failover.Upstreams) > 0
}

func (f *FailoverFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	log := log.FromContext(ctx)
	log.Info("Applying failover feature", "route", builder.GetRoute().Name)

	routingConfigs := builder.RoutingConfigs()
	route := builder.GetRoute()
	failover := route.Spec.Traffic.Failover
	envName := contextutil.EnvFromContextOrDie(ctx)

	builder.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))

	// This is the proxy upstream that should be used in all non-failover cases.
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
	}
	routingConfigs.Add(proxyRoutingCfg)

	// The failover upstreams are used if the Jumper determines that the primary upstream is not available.
	// It does so by checking the health of the TargetZoneName using the ZoneHealthCheckService.
	for _, failoverUpstream := range failover.Upstreams {
		routingCfg := &plugin.RoutingConfig{
			RemoteApiUrl:   failoverUpstream.Url(),
			ApiBasePath:    failoverUpstream.Path,
			Realm:          route.Spec.Realm.Name,
			Issuer:         failoverUpstream.IssuerUrl,
			ClientId:       failoverUpstream.ClientId,
			ClientSecret:   failoverUpstream.ClientSecret,
			Environment:    envName,
			TargetZoneName: "", // In case of failover, the zone is not set
		}
		routingConfigs.Add(routingCfg)
	}

	return nil
}
