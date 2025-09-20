// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"reflect"

	"github.com/telekom/controlplane/rover-server/internal/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func mapExposureTraffic(in api.ApiExposure, out *roverv1.ApiExposure) {
	// Initialize Traffic if needed
	if out.Traffic == nil {
		out.Traffic = &roverv1.Traffic{}
	}

	// Handle Failover.Zones - safely extract zones from possibly nil struct
	var failoverZones []string
	// First check if Failover struct is initialized
	if !reflect.ValueOf(in.Failover).IsZero() {
		failoverZones = in.Failover.Zones
	}

	// Only create a Failover object if we have zones
	if len(failoverZones) > 0 {
		out.Traffic.Failover = &roverv1.Failover{
			Zones: failoverZones,
		}
	}

	// CircuitBreaker configuration
	mapCircuitBreaker(in, out)

	// Map rate limiting configuration
	mapRateLimit(in, out)
}

// mapRateLimit maps the rate limiting configuration from api.ApiExposure to roverv1.ApiExposure
func mapRateLimit(in api.ApiExposure, out *roverv1.ApiExposure) {
	// Check if RateLimit is initialized to avoid nil pointer panics
	if reflect.ValueOf(in.RateLimit).IsZero() {
		return
	}

	// Check if any rate limiting is configured
	hasProviderLimit := in.RateLimit.Provider.Second != 0 || in.RateLimit.Provider.Minute != 0 || in.RateLimit.Provider.Hour != 0
	hasConsumerDefaultLimit := in.RateLimit.ConsumerDefault.Second != 0 || in.RateLimit.ConsumerDefault.Minute != 0 || in.RateLimit.ConsumerDefault.Hour != 0
	hasConsumerOverrides := len(in.RateLimit.Consumers) > 0

	// If no rate limiting is configured, return without setting anything
	if !hasProviderLimit && !hasConsumerDefaultLimit && !hasConsumerOverrides {
		return
	}

	// RateLimit objects if needed - Traffic is already initialized
	if out.Traffic.RateLimit == nil {
		out.Traffic.RateLimit = &roverv1.RateLimit{}
	}

	// Map provider rate limits if configured
	if hasProviderLimit {
		out.Traffic.RateLimit.Provider = &roverv1.RateLimitConfig{
			Limits: &roverv1.Limits{
				Second: int(in.RateLimit.Provider.Second),
				Minute: int(in.RateLimit.Provider.Minute),
				Hour:   int(in.RateLimit.Provider.Hour),
			},
			Options: roverv1.RateLimitOptions{
				FaultTolerant:     in.RateLimit.Provider.FaultTolerant,
				HideClientHeaders: in.RateLimit.Provider.HideClientHeaders,
			},
		}
	}

	// Map consumer rate limits if configured
	if hasConsumerDefaultLimit || hasConsumerOverrides {
		out.Traffic.RateLimit.Consumers = &roverv1.ConsumerRateLimits{}

		// Map consumer default limits if configured
		if hasConsumerDefaultLimit {
			out.Traffic.RateLimit.Consumers.Default = &roverv1.ConsumerRateLimitDefaults{
				Limits: roverv1.Limits{
					Second: int(in.RateLimit.ConsumerDefault.Second),
					Minute: int(in.RateLimit.ConsumerDefault.Minute),
					Hour:   int(in.RateLimit.ConsumerDefault.Hour),
				},
				// ConsumerRateLimitDefaults doesn't have FaultTolerant and HideClientHeaders fields in the API
			}
		}

		// Map consumer overrides if configured
		if hasConsumerOverrides {
			overrides := make([]roverv1.ConsumerRateLimitOverrides, len(in.RateLimit.Consumers))
			for i, consumer := range in.RateLimit.Consumers {
				overrides[i] = roverv1.ConsumerRateLimitOverrides{
					// ConsumerRateLimit in the API now has an 'id' field
					Consumer: consumer.Id, // The field name is now "id" in the API
					Limits: roverv1.Limits{
						Second: int(consumer.Second),
						Minute: int(consumer.Minute),
						Hour:   int(consumer.Hour),
					},
					// ConsumerRateLimitOverrides doesn't have FaultTolerant and HideClientHeaders fields in the API
				}
			}
			out.Traffic.RateLimit.Consumers.Overrides = overrides
		}
	}
}

func mapCircuitBreaker(in api.ApiExposure, out *roverv1.ApiExposure) {
	// Check if CircuitBreaker is initialized to avoid nil pointer panics
	if reflect.ValueOf(in.CircuitBreaker).IsZero() {
		return
	}

	// initialize if needed
	if out.Traffic == nil {
		out.Traffic = &roverv1.Traffic{}
	}

	if out.Traffic.CircuitBreaker == nil {
		out.Traffic.CircuitBreaker = &roverv1.CircuitBreaker{}
	}

	out.Traffic.CircuitBreaker = &roverv1.CircuitBreaker{
		Enabled: in.CircuitBreaker.Enabled,
	}
}
