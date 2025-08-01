// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"

	rover "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func HandleExposure(ctx context.Context, c client.JanitorClient, owner *rover.Rover, exp *rover.ApiExposure) error {

	log := log.FromContext(ctx)
	log.V(1).Info("Handle APIExposure", "basePath", exp.BasePath)

	name := MakeName(owner.Name, exp.BasePath, "")

	apiExposure := &apiapi.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: owner.Namespace,
		},
	}

	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	mutator := func() error {
		err := controllerutil.SetControllerReference(owner, apiExposure, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}
		apiExposure.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeValue(exp.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
		}

		apiExposure.Spec = apiapi.ApiExposureSpec{
			ApiBasePath:    exp.BasePath,
			Visibility:     apiapi.Visibility(exp.Visibility.String()),
			Approval:       apiapi.ApprovalStrategy(exp.Approval.Strategy),
			Zone:           zoneRef,
			Upstreams:      make([]apiapi.Upstream, len(exp.Upstreams)),
			Security:       mapSecurityToApiSecurity(exp.Security),
			Transformation: mapTransformationToApiTransformation(exp.Transformation),
			Traffic:        mapTrafficToApiTraffic(environment, exp.Traffic),
		}

		for i, upstream := range exp.Upstreams {
			apiExposure.Spec.Upstreams[i] = apiapi.Upstream{
				Url:    upstream.URL,
				Weight: upstream.Weight,
			}
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, apiExposure, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update ApiExposure")
	}

	owner.Status.ApiExposures = append(owner.Status.ApiExposures, types.ObjectRef{
		Name:      apiExposure.Name,
		Namespace: apiExposure.Namespace,
	})
	return err
}

// security
func mapSecurityToApiSecurity(roverSecurity *rover.Security) *apiapi.Security {
	if roverSecurity == nil {
		return nil
	}

	security := &apiapi.Security{}

	if roverSecurity.M2M != nil {
		security.M2M = &apiapi.Machine2MachineAuthentication{
			Scopes: roverSecurity.M2M.Scopes,
		}

		if roverSecurity.M2M.ExternalIDP != nil {
			security.M2M.ExternalIDP = &apiapi.ExternalIdentityProvider{
				TokenEndpoint: roverSecurity.M2M.ExternalIDP.TokenEndpoint,
				TokenRequest:  roverSecurity.M2M.ExternalIDP.TokenRequest,
				GrantType:     roverSecurity.M2M.ExternalIDP.GrantType,
				Client:        toApiClient(roverSecurity.M2M.ExternalIDP.Client),
				Basic:         toApiBasic(roverSecurity.M2M.ExternalIDP.Basic),
			}
		}

		if roverSecurity.M2M.Basic != nil {
			security.M2M.Basic = &apiapi.BasicAuthCredentials{
				Username: roverSecurity.M2M.Basic.Username,
				Password: roverSecurity.M2M.Basic.Password,
			}
		}
	}

	return security

}

// transformation
func mapTransformationToApiTransformation(roverTransformation *rover.Transformation) *apiapi.Transformation {
	if roverTransformation == nil {
		return nil
	}

	apiTransformation := &apiapi.Transformation{}

	if len(roverTransformation.Request.Headers.Remove) > 0 {
		apiTransformation.Request.Headers.Remove = roverTransformation.Request.Headers.Remove
	}

	return apiTransformation
}

// traffic
func mapTrafficToApiTraffic(env string, roverTraffic *rover.Traffic) apiapi.Traffic {
	if roverTraffic == nil {
		return apiapi.Traffic{}
	}

	apiTraffic := apiapi.Traffic{}

	// Handle failover
	failoverZones, hasFailover := getFailoverZones(env, roverTraffic.Failover)
	if hasFailover {
		apiTraffic.Failover = &apiapi.Failover{
			Zones: failoverZones,
		}
	}

	// Handle rate limits
	if roverTraffic.RateLimit != nil {
		apiTraffic.RateLimit = mapRateLimitToApiRateLimit(roverTraffic.RateLimit.Provider)

		if roverTraffic.RateLimit.Consumers != nil {
			apiTraffic.SubscriberRateLimit = mapConsumerRateLimitToApiRateLimit(roverTraffic.RateLimit.Consumers)
		}
	}

	return apiTraffic
}

func mapRateLimitToApiRateLimit(roverRateLimitConfig *rover.RateLimitConfig) *apiapi.RateLimit {
	var rateLimitConfig *apiapi.RateLimit

	if roverRateLimitConfig != nil {
		rateLimitConfig = &apiapi.RateLimit{
			Limits: apiapi.LimitConfig{
				Second: roverRateLimitConfig.Limits.Second,
				Minute: roverRateLimitConfig.Limits.Minute,
				Hour:   roverRateLimitConfig.Limits.Hour,
			},
			Options: apiapi.LimitOptions{
				HideClientHeaders: roverRateLimitConfig.Options.HideClientHeaders,
				FaultTolerant:     roverRateLimitConfig.Options.FaultTolerant,
			},
		}
	}
	return rateLimitConfig
}

func mapConsumerRateLimitToApiRateLimit(consumerRateLimits *rover.ConsumerRateLimits) *apiapi.SubscriberRateLimit {
	if consumerRateLimits == nil || consumerRateLimits.Default == nil {
		return nil
	}

	overrides := map[string]apiapi.RateLimit{}
	for k, v := range consumerRateLimits.Overrides {
		overrides[k] = *mapRateLimitToApiRateLimit(&v)
	}

	// For subscriber rate limits, we use the default consumer rate limit
	return &apiapi.SubscriberRateLimit{
		Default:   mapRateLimitToApiRateLimit(consumerRateLimits.Default),
		Overrides: &overrides,
	}
}
