// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

func HandleExposure(ctx context.Context, c client.JanitorClient, owner *rover.Rover, exp *rover.ApiExposure) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Handle APIExposure", "basePath", exp.BasePath)

	name := MakeName(owner.Name, exp.BasePath, "")

	apiExposure := &apiapi.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(name),
			Namespace: owner.Namespace,
		},
	}

	environment := contextutil.EnvFromContextOrDie(ctx)
	zoneRef := types.ObjectRef{
		Name:      owner.Spec.Zone,
		Namespace: environment,
	}

	// Resolve the owning team up front; it is added to the trusted teams below.
	// The owner team is required here, so block (and requeue) if it cannot be resolved.
	ownerTeam, err := organizationv1.FindTeamForObject(ctx, owner)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrlerrors.BlockedErrorf("owner team not found for application %s", owner.Name)
		}
		return err
	}

	mutator := func() error {
		err = controllerutil.SetControllerReference(owner, apiExposure, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}
		apiExposure.Labels = map[string]string{
			apiapi.BasePathLabelKey:             labelutil.NormalizeLabelValue(exp.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeValue(owner.Name),
		}

		apiExposure.Spec = apiapi.ApiExposureSpec{
			ApiBasePath: exp.BasePath,
			Visibility:  apiapi.Visibility(exp.Visibility.String()),
			Approval: apiapi.Approval{
				Strategy: apiapi.ApprovalStrategy(exp.Approval.Strategy),
			},
			Zone:           zoneRef,
			Upstreams:      make([]apiapi.Upstream, len(exp.Upstreams)),
			Security:       mapSecurityToApiSecurity(exp.Security),
			Transformation: mapTransformationToApiTransformation(exp.Transformation),
			Traffic:        mapTrafficToApiTraffic(environment, exp.Traffic),
		}

		apiExposure.Spec.Approval.TrustedTeams, err = mapTrustedTeamsToApiTrustedTeams(ctx, exp.Approval.TrustedTeams)
		if err != nil {
			return errors.Wrap(err, "failed to map trusted teams")
		}

		// add owner to trusted teams (already resolved above)
		apiExposure.Spec.Approval.TrustedTeams = append(apiExposure.Spec.Approval.TrustedTeams, ownerTeam.GetName())

		for i, upstream := range exp.Upstreams {
			apiExposure.Spec.Upstreams[i] = apiapi.Upstream{
				Url:    upstream.URL,
				Weight: upstream.Weight,
			}
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, apiExposure, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update ApiExposure")
	}

	owner.Status.ApiExposures = append(owner.Status.ApiExposures, types.ObjectRef{
		Name:      apiExposure.Name,
		Namespace: apiExposure.Namespace,
	})
	return nil
}

func mapTrustedTeamsToApiTrustedTeams(ctx context.Context, teams []rover.TrustedTeam) ([]string, error) {
	logger := log.FromContext(ctx)
	if len(teams) == 0 {
		return nil, nil
	}

	apiTrustedTeams := make([]string, 0, len(teams))
	for _, team := range teams {
		namespace := contextutil.EnvFromContextOrDie(ctx) + "--" + team.Group + "--" + team.Team
		t, err := organizationv1.FindTeamForNamespace(ctx, namespace)
		switch {
		case err != nil && apierrors.IsNotFound(err):
			logger.Info(fmt.Sprintf("Trusted team %s/%s not found", team.Group, team.Team))
		case err != nil:
			return nil, err
		default:
			apiTrustedTeams = append(apiTrustedTeams, t.GetName())
		}
	}

	return apiTrustedTeams, nil
}

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
				TokenRequest:  apiapi.TokenRequestMethod(roverSecurity.M2M.ExternalIDP.TokenRequest),
				GrantType:     apiapi.GrantType(roverSecurity.M2M.ExternalIDP.GrantType),
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

		security.M2M.Claims = mapClaimsToApiClaims(roverSecurity.M2M.Claims)
	}

	return security
}

// mapClaimsToApiClaims forwards the claim to the api domain. A user-provided literal
// is copied through; ValueFrom sources (ProviderClientId, BasePath, ConsumerClientId)
// stay symbolic and are resolved in the api domain (or by Jumper for ConsumerClientId).
func mapClaimsToApiClaims(roverClaims *rover.Claims) *apiapi.Claims {
	if roverClaims == nil || roverClaims.Aud == nil {
		return nil
	}

	aud := roverClaims.Aud
	resolved := &apiapi.Claim{}

	switch {
	case aud.Value != "":
		resolved.Value = aud.Value
	case aud.ValueFrom != "":
		resolved.ValueFrom = apiapi.ClaimValueFrom(aud.ValueFrom)
	default:
		return nil
	}

	return &apiapi.Claims{Aud: resolved}
}

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

func mapTrafficToApiTraffic(env string, roverTraffic *rover.Traffic) apiapi.Traffic {
	if roverTraffic == nil {
		return apiapi.Traffic{}
	}

	apiTraffic := apiapi.Traffic{}

	// Handle failover
	failoverZones, hasFailover := mapProviderFailoverZones(env, roverTraffic.Failover)
	if hasFailover {
		apiTraffic.Failover = &apiapi.ProviderFailover{
			Zones: failoverZones,
		}
	}

	if roverTraffic.HasRateLimit() {
		apiTraffic.RateLimit = &apiapi.RateLimit{}
	}

	// Handle rate limits
	if roverTraffic.HasProviderRateLimit() {
		apiTraffic.RateLimit.Provider = mapRateLimitConfigToApiRateLimitConfig(roverTraffic.RateLimit.Provider)
	}

	if roverTraffic.HasConsumerRateLimit() {
		apiTraffic.RateLimit.SubscriberRateLimit = mapConsumerRateLimitToApiSubscriberRateLimit(roverTraffic.RateLimit.Consumers)
	}

	if roverTraffic.HasCircuitBreaker() {
		apiTraffic.CircuitBreaker = mapCircuitBreakerToApiCircuitBreaker(roverTraffic.CircuitBreaker)
	}

	return apiTraffic
}

func mapCircuitBreakerToApiCircuitBreaker(breaker *rover.CircuitBreaker) *apiapi.CircuitBreaker {
	if breaker == nil {
		return nil
	}

	return &apiapi.CircuitBreaker{
		Enabled: breaker.Enabled,
	}
}

func mapRateLimitConfigToApiRateLimitConfig(roverRateLimitConfig *rover.RateLimitConfig) *apiapi.RateLimitConfig {
	if roverRateLimitConfig == nil {
		return nil
	}

	return &apiapi.RateLimitConfig{
		Limits: apiapi.Limits{
			Second: roverRateLimitConfig.Limits.Second,
			Minute: roverRateLimitConfig.Limits.Minute,
			Hour:   roverRateLimitConfig.Limits.Hour,
		},
		Options: apiapi.RateLimitOptions{
			HideClientHeaders: roverRateLimitConfig.Options.HideClientHeaders,
			FaultTolerant:     roverRateLimitConfig.Options.FaultTolerant,
		},
	}
}

func mapConsumerRateLimitDefaultsToApiSubscriberRateLimitDefaults(roverRateLimitConfig *rover.ConsumerRateLimitDefaults) *apiapi.SubscriberRateLimitDefaults {
	if roverRateLimitConfig == nil {
		return nil
	}

	return &apiapi.SubscriberRateLimitDefaults{
		Limits: apiapi.Limits{
			Second: roverRateLimitConfig.Limits.Second,
			Minute: roverRateLimitConfig.Limits.Minute,
			Hour:   roverRateLimitConfig.Limits.Hour,
		},
	}
}

func mapConsumerRateLimitToApiSubscriberRateLimit(consumerRateLimits *rover.ConsumerRateLimits) *apiapi.SubscriberRateLimits {
	if consumerRateLimits == nil {
		return nil
	}

	subscriberRateLimits := &apiapi.SubscriberRateLimits{
		Default: mapConsumerRateLimitDefaultsToApiSubscriberRateLimitDefaults(consumerRateLimits.Default),
	}

	if len(consumerRateLimits.Overrides) > 0 {
		var overrides []apiapi.RateLimitOverrides
		for _, override := range consumerRateLimits.Overrides {
			overrides = append(overrides, apiapi.RateLimitOverrides{
				Subscriber: override.Consumer,
				Limits: apiapi.Limits{
					Second: override.Limits.Second,
					Minute: override.Limits.Minute,
					Hour:   override.Limits.Hour,
				},
			})
		}
		subscriberRateLimits.Overrides = overrides
	}
	return subscriberRateLimits
}
