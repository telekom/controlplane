// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature/config"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ features.Feature = &CircuitBreakerFeature{}

type CircuitBreakerFeature struct {
	priority int
}

var InstanceCircuitBreakerFeature = &CircuitBreakerFeature{
	priority: InstanceLastMileSecurityFeature.priority - 1,
}

func (c CircuitBreakerFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeCircuitBreaker
}

func (c CircuitBreakerFeature) Priority() int {
	return c.priority
}

// IsUsed as per ARD0014, CB is an opt-in feature, but the cleanup mechanism is special, since CB is not a plugin - see comments inside the function
func (c CircuitBreakerFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		// assume that CB is not used if there is no route
		return false
	}

	if route.IsProxy() {
		return false
	}

	if route.Spec.Traffic.CircuitBreaker {
		return true
	} else if route.GetUpstreamId() != "" {
		// this means that CB was previously configured, but now is disabled
		// we return true here, because the Apply function will do the cleanup
		// this is NOT a typical usecase, but since CB is special (not a plugin) we handle it this way

		return true
	}

	return false
}

func (c CircuitBreakerFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Configuring CircuitBreaker", "name", c.Name())

	route, ok := builder.GetRoute()
	if !ok {
		return fmt.Errorf("cannot find route")
	}

	routeName := route.GetName()
	kongClient := builder.GetKongClient()
	kongAdminApi := kongClient.GetKongAdminApi()

	if isDeleteScenario(route) {
		builder.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))

		kongUpstreamName := route.GetUpstreamId()
		upstreamDeleteResponse, err := kongAdminApi.DeleteUpstreamWithResponse(ctx, kongUpstreamName)
		if err != nil {
			return errors.Wrapf(err, "failed to delete upstream for route %s", routeName)
		}
		if err := client.CheckStatusCode(upstreamDeleteResponse, 200, 204); err != nil {
			return errors.Wrapf(fmt.Errorf("error body from kong admin api: %s", string(upstreamDeleteResponse.Body)), "failed to create upstream for route %s", routeName)
		}

	} else {
		// ! important - if CB is enabled then the kong service value needs to reference the kong upstream (same name as route name)
		builder.SetUpstream(&client.CustomUpstream{
			Scheme: "http",
			Host:   routeName,
			Port:   8080,
			Path:   "/proxy",
		})

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
			Tags: &[]string{
				client.BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
				client.BuildTag("upstream", upstreamName),
			},
		}

		upstreamResponse, err := kongAdminApi.UpsertUpstreamWithResponse(ctx, upstreamName, upstreamBody)
		if err != nil {
			return errors.Wrap(err, "failed to create upstream")
		}
		if err := client.CheckStatusCode(upstreamResponse, 200); err != nil {
			return errors.Wrap(fmt.Errorf("error body from kong admin api: %s", string(upstreamResponse.Body)), "failed to create upstream")
		}
		log.V(1).Info("upstream response", "response", upstreamResponse)
		if upstreamResponse.JSON200 != nil && upstreamResponse.JSON200.Id != nil {
			route.SetUpstreamId(*upstreamResponse.JSON200.Id)
		} else {
			route.SetUpstreamId(upstreamName)
		}

		targetsName := routeName
		targetsTarget := "localhost:8080"
		targetsWeight := 100
		targetsBody := kong.CreateTargetForUpstreamJSONRequestBody{
			Tags: &[]string{
				client.BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
				client.BuildTag("targets", targetsName),
			},
			Target: &targetsTarget,
			Weight: &targetsWeight,
		}

		// this is a special case with the kong admin API - this endpoint /upstreams/:upstreamName/targets actually accepts multiple POST requests, so this is not a mistake
		targetsResponse, err := kongAdminApi.CreateTargetForUpstreamWithResponse(ctx, upstreamName, targetsBody)
		if err != nil {
			return errors.Wrap(err, "failed to create targets for upstream")
		}
		if err := client.CheckStatusCode(targetsResponse, 200, 201); err != nil {
			return errors.Wrap(fmt.Errorf("error body from kong admin api: %s", string(targetsResponse.Body)), "failed to create targets for upstream")
		}
		log.V(1).Info("targets response", "response", targetsResponse)
		if targetsResponse.JSON200 != nil && targetsResponse.JSON200.Id != nil {
			route.SetTargetsId(*targetsResponse.JSON200.Id)
		} else {
			route.SetTargetsId("targets id missing from kong response")
		}
	}

	return nil
}

func isDeleteScenario(route *gatewayv1.Route) bool {
	if !route.Spec.Traffic.CircuitBreaker && route.GetUpstreamId() != "" {
		return true
	} else {
		return false
	}
}
