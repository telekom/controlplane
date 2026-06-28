// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

// HandleExposure creates or updates an McpExposure resource owned by the given Rover.
func HandleExposure(ctx context.Context, c client.JanitorClient, owner *rover.Rover, exp *rover.AiExposure) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Handle AiExposure", "basePath", exp.BasePath)

	name := MakeName(owner.Name, exp.BasePath)

	mcpExposure := &agenticv1.McpExposure{
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

	mutator := func() error {
		err := controllerutil.SetControllerReference(owner, mcpExposure, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		mcpExposure.Labels = map[string]string{
			agenticv1.McpBasePathLabelKey:       labelutil.NormalizeLabelValue(exp.BasePath),
			config.BuildLabelKey("zone"):        labelutil.NormalizeLabelValue(zoneRef.Name),
			config.BuildLabelKey("application"): labelutil.NormalizeLabelValue(owner.Name),
		}

		trustedTeams, err := mapTrustedTeams(ctx, exp.Approval.TrustedTeams)
		if err != nil {
			return errors.Wrap(err, "failed to map trusted teams")
		}

		ownerTeam, err := organizationv1.FindTeamForObject(ctx, owner)
		switch {
		case err != nil && apierrors.IsNotFound(err):
			logger.Info(fmt.Sprintf("Team not found for application %s, err: %v", owner.Name, err))
		case err != nil:
			return err
		default:
			trustedTeams = append(trustedTeams, ownerTeam.GetName())
		}

		mcpExposure.Spec = agenticv1.McpExposureSpec{
			BasePath:   exp.BasePath,
			Upstreams:  mapUpstreams(exp.Upstreams),
			Visibility: agenticv1.Visibility(exp.Visibility.String()),
			Approval: agenticv1.Approval{
				Strategy:     agenticv1.ApprovalStrategy(exp.Approval.Strategy),
				TrustedTeams: trustedTeams,
			},
			Zone:           zoneRef,
			Provider:       *owner.Status.Application,
			Variant:        agenticv1.McpVariant(exp.Variant),
			Security:       mapSecurityToAgenticSecurity(exp.Security),
			Traffic:        mapTrafficToAgenticTraffic(environment, exp.Traffic),
			Transformation: mapTransformationToAgenticTransformation(exp.Transformation),
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, mcpExposure, mutator)
	if err != nil {
		return errors.Wrap(err, "failed to create or update McpExposure")
	}

	owner.Status.AiExposures = append(owner.Status.AiExposures, types.ObjectRef{
		Name:      mcpExposure.Name,
		Namespace: mcpExposure.Namespace,
	})
	return nil
}

// mapTrustedTeams resolves rover TrustedTeam references (Group/Team) to team resource names.
func mapTrustedTeams(ctx context.Context, teams []rover.TrustedTeam) ([]string, error) {
	logger := log.FromContext(ctx)
	if len(teams) == 0 {
		return nil, nil
	}

	resolved := make([]string, 0, len(teams))
	for _, team := range teams {
		namespace := contextutil.EnvFromContextOrDie(ctx) + "--" + team.Group + "--" + team.Team
		t, err := organizationv1.FindTeamForNamespace(ctx, namespace)
		switch {
		case err != nil && apierrors.IsNotFound(err):
			logger.Info(fmt.Sprintf("Trusted team %s/%s not found", team.Group, team.Team))
		case err != nil:
			return nil, err
		default:
			resolved = append(resolved, t.GetName())
		}
	}

	return resolved, nil
}

// mapUpstreams converts rover Upstream types to agentic Upstream types.
func mapUpstreams(roverUpstreams []rover.Upstream) []agenticv1.Upstream {
	upstreams := make([]agenticv1.Upstream, len(roverUpstreams))
	for i, u := range roverUpstreams {
		upstreams[i] = agenticv1.Upstream{
			Url:    u.URL,
			Weight: u.Weight,
		}
	}
	return upstreams
}

// mapSecurityToAgenticSecurity converts rover Security to agentic Security.
func mapSecurityToAgenticSecurity(roverSecurity *rover.Security) *agenticv1.Security {
	if roverSecurity == nil {
		return nil
	}

	security := &agenticv1.Security{}

	if roverSecurity.M2M != nil {
		security.M2M = mapM2MToAgenticM2M(roverSecurity.M2M)
	}

	return security
}

// mapM2MToAgenticM2M converts rover M2M auth to agentic M2M auth.
func mapM2MToAgenticM2M(m2m *rover.Machine2MachineAuthentication) *agenticv1.Machine2MachineAuthentication {
	result := &agenticv1.Machine2MachineAuthentication{
		Scopes: m2m.Scopes,
	}

	if m2m.ExternalIDP != nil {
		result.ExternalIDP = &agenticv1.ExternalIdentityProvider{
			TokenEndpoint: m2m.ExternalIDP.TokenEndpoint,
			TokenRequest:  agenticv1.TokenRequestMethod(m2m.ExternalIDP.TokenRequest),
			GrantType:     m2m.ExternalIDP.GrantType,
		}
		if m2m.ExternalIDP.Client != nil {
			result.ExternalIDP.Client = &agenticv1.OAuth2ClientCredentials{
				ClientId:     m2m.ExternalIDP.Client.ClientId,
				ClientSecret: m2m.ExternalIDP.Client.ClientSecret,
				ClientKey:    m2m.ExternalIDP.Client.ClientKey,
			}
		}
		if m2m.ExternalIDP.Basic != nil {
			result.ExternalIDP.Basic = &agenticv1.BasicAuthCredentials{
				Username: m2m.ExternalIDP.Basic.Username,
				Password: m2m.ExternalIDP.Basic.Password,
			}
		}
	}

	if m2m.Basic != nil {
		result.Basic = &agenticv1.BasicAuthCredentials{
			Username: m2m.Basic.Username,
			Password: m2m.Basic.Password,
		}
	}

	return result
}

// mapTrafficToAgenticTraffic converts rover Traffic to agentic Traffic.
func mapTrafficToAgenticTraffic(env string, roverTraffic *rover.Traffic) agenticv1.Traffic {
	if roverTraffic == nil {
		return agenticv1.Traffic{}
	}

	traffic := agenticv1.Traffic{}

	if roverTraffic.HasCircuitBreaker() {
		traffic.CircuitBreaker = &agenticv1.CircuitBreaker{
			Enabled: roverTraffic.CircuitBreaker.Enabled,
		}
	}

	if roverTraffic.HasRateLimit() {
		rateLimit := &agenticv1.RateLimit{}

		if roverTraffic.HasProviderRateLimit() && roverTraffic.RateLimit.Provider.Limits != nil {
			rateLimit.Provider = &agenticv1.RateLimitConfig{
				Limits: agenticv1.Limits{
					Second: roverTraffic.RateLimit.Provider.Limits.Second,
					Minute: roverTraffic.RateLimit.Provider.Limits.Minute,
					Hour:   roverTraffic.RateLimit.Provider.Limits.Hour,
				},
				Options: agenticv1.RateLimitOptions{
					HideClientHeaders: roverTraffic.RateLimit.Provider.Options.HideClientHeaders,
					FaultTolerant:     roverTraffic.RateLimit.Provider.Options.FaultTolerant,
				},
			}
		}

		if roverTraffic.HasConsumerRateLimit() {
			rateLimit.SubscriberRateLimit = mapConsumerRateLimitToSubscriberRateLimit(roverTraffic.RateLimit.Consumers)
		}

		traffic.RateLimit = rateLimit
	}

	if roverTraffic.HasFailover() {
		failoverZones := make([]types.ObjectRef, 0, len(roverTraffic.Failover.Zones))
		for _, zone := range roverTraffic.Failover.Zones {
			failoverZones = append(failoverZones, types.ObjectRef{
				Name:      zone,
				Namespace: env,
			})
		}
		traffic.Failover = &agenticv1.Failover{
			Zones: failoverZones,
		}
	}

	return traffic
}

// mapConsumerRateLimitToSubscriberRateLimit converts rover ConsumerRateLimits to agentic SubscriberRateLimits.
func mapConsumerRateLimitToSubscriberRateLimit(consumerRateLimits *rover.ConsumerRateLimits) *agenticv1.SubscriberRateLimits {
	if consumerRateLimits == nil {
		return nil
	}

	subscriberRateLimits := &agenticv1.SubscriberRateLimits{}

	if consumerRateLimits.Default != nil {
		subscriberRateLimits.Default = &agenticv1.SubscriberRateLimitDefaults{
			Limits: agenticv1.Limits{
				Second: consumerRateLimits.Default.Limits.Second,
				Minute: consumerRateLimits.Default.Limits.Minute,
				Hour:   consumerRateLimits.Default.Limits.Hour,
			},
		}
	}

	if len(consumerRateLimits.Overrides) > 0 {
		overrides := make([]agenticv1.RateLimitOverrides, len(consumerRateLimits.Overrides))
		for i, override := range consumerRateLimits.Overrides {
			overrides[i] = agenticv1.RateLimitOverrides{
				Subscriber: override.Consumer,
				Limits: agenticv1.Limits{
					Second: override.Limits.Second,
					Minute: override.Limits.Minute,
					Hour:   override.Limits.Hour,
				},
			}
		}
		subscriberRateLimits.Overrides = overrides
	}

	return subscriberRateLimits
}

// mapTransformationToAgenticTransformation converts rover Transformation to agentic Transformation.
func mapTransformationToAgenticTransformation(roverTransformation *rover.Transformation) *agenticv1.Transformation {
	if roverTransformation == nil {
		return nil
	}

	agenticTransformation := &agenticv1.Transformation{}

	if len(roverTransformation.Request.Headers.Remove) > 0 {
		agenticTransformation.Request.Headers.Remove = roverTransformation.Request.Headers.Remove
	}

	return agenticTransformation
}
