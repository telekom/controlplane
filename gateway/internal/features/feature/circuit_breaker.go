// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature/config"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ features.Feature = &CircuitBreakerFeature{}

// CircuitBreakerPriority The priority is the highest of all features as it modifies the Kong service host and to make sure it will not interfere with any other feature.
const CircuitBreakerPriority = 110

// DefaultTargetsTarget The name sounds a bit misleading, but the Kong entity is called Target, and it has a target field
const DefaultTargetsTarget = "localhost:8080"

type CircuitBreakerFeature struct {
	priority int
}

var InstanceCircuitBreakerFeature = &CircuitBreakerFeature{
	priority: CircuitBreakerPriority,
}

func (c CircuitBreakerFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeCircuitBreaker
}

func (c CircuitBreakerFeature) Priority() int {
	return c.priority
}

// IsUsed as per ARD0014, CB is an opt-in feature, but the cleanup mechanism is special, since CB is not a plugin.
// A disabled or removed CB is used for the cleanup if its upstream may exist: an ID is stored, or the route has a
// name prefix, whose upstream is found by discovery even if the status lost its IDs.
func (c CircuitBreakerFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		// assume that CB is not used if there is no route
		return false
	}

	if route.IsProxy() {
		return false
	}

	if !isDeleteScenario(route) {
		return true
	}
	return route.GetUpstreamId() != "" || route.GetTargetsId() != "" || route.GetNamePrefix() != ""
}

func (c CircuitBreakerFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Configuring CircuitBreaker", "name", c.Name())

	route, ok := builder.GetRoute()
	if !ok {
		return fmt.Errorf("cannot find route")
	}

	if isDeleteScenario(route) {
		handleDeletion(builder)
		return nil
	}
	handleApply(ctx, builder, route)
	return nil
}

// handleDeletion makes the Kong client delete the upstream after the service no longer points to it.
func handleDeletion(builder features.FeaturesBuilder) {
	current := builder.GetUpstream()
	if current == nil {
		current = client.NewUpstreamOrDie(plugin.LocalhostProxyUrl)
	}
	builder.SetUpstream(&client.UpstreamRemoval{Upstream: current})
}

// isDeleteScenario reports whether the CB is disabled or removed.
func isDeleteScenario(route *gatewayv1.Route) bool {
	cb := route.Spec.Traffic.CircuitBreaker
	return cb == nil || !cb.Enabled
}

// handleApply makes the Kong client write the upstream and target the service points to.
func handleApply(ctx context.Context, builder features.FeaturesBuilder, route *gatewayv1.Route) {
	routeName := route.KongName()

	upstreamAlgorithm := kong.RoundRobin
	passiveHealthcheckType := kong.CreateUpstreamRequestHealthchecksPassiveTypeHttp
	activeHealthcheckType := kong.CreateUpstreamRequestHealthchecksActiveTypeHttp
	upstreamName := routeName
	upstreamBody := kong.CreateUpstreamJSONRequestBody{
		Algorithm: &upstreamAlgorithm,
		Name:      upstreamName,
		Healthchecks: &kong.CreateUpstreamRequestHealthchecks{
			Active: &kong.CreateUpstreamRequestHealthchecksActive{
				Healthy: &kong.CreateUpstreamRequestHealthchecksActiveHealthy{
					HttpStatuses: &config.DefaultCircuitBreaker.Active.HealthyHttpStatuses,
				},
				Type: &activeHealthcheckType,
				Unhealthy: &kong.CreateUpstreamRequestHealthchecksActiveUnhealthy{
					HttpStatuses: &config.DefaultCircuitBreaker.Active.UnhealthyHttpStatuses,
				},
			},
			Passive: &kong.CreateUpstreamRequestHealthchecksPassive{
				Healthy: &kong.CreateUpstreamRequestHealthchecksPassiveHealthy{
					HttpStatuses: config.ToPassiveHealthyHttpStatuses(config.DefaultCircuitBreaker.Passive.HealthyHttpStatuses),
					Successes:    &config.DefaultCircuitBreaker.Passive.HealthySuccesses,
				},
				Type: &passiveHealthcheckType,
				Unhealthy: &kong.CreateUpstreamRequestHealthchecksPassiveUnhealthy{
					HttpFailures: &config.DefaultCircuitBreaker.Passive.UnhealthyHttpFailures,
					HttpStatuses: config.ToPassiveUnhealthyHttpStatuses(config.DefaultCircuitBreaker.Passive.UnhealthyHttpStatuses),
					TcpFailures:  &config.DefaultCircuitBreaker.Passive.UnhealthyTcpFailures,
					Timeouts:     &config.DefaultCircuitBreaker.Passive.UnhealthyTimeouts,
				},
			},
		},
		Tags: realmTags(route,
			client.BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
			client.BuildTag("upstream", upstreamName),
			client.BuildTag("route", route.GetName()),
		),
	}

	targetsName := routeName
	targetsTarget := DefaultTargetsTarget
	targetsWeight := 100
	targetsBody := kong.CreateTargetForUpstreamJSONRequestBody{
		Tags: realmTags(route,
			client.BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
			client.BuildTag("targets", targetsName),
			client.BuildTag("route", route.GetName()),
		),
		Target: &targetsTarget,
		Weight: &targetsWeight,
	}

	// ! important - if CB is enabled then the kong service value needs to reference the kong upstream (same name as the Kong route)
	builder.SetUpstream(&client.CircuitBreakerUpstream{
		CustomUpstream: client.CustomUpstream{Scheme: "http", Host: routeName, Port: 8080, Path: "/proxy"},
		Upstream:       &upstreamBody,
		Target:         &targetsBody,
	})
}

// realmTags appends realm--<namePrefix> to the tags if the route has a name prefix.
func realmTags(route *gatewayv1.Route, tags ...string) *[]string {
	if prefix := route.GetNamePrefix(); prefix != "" {
		tags = append(tags, client.BuildTag("realm", prefix))
	}
	return &tags
}
