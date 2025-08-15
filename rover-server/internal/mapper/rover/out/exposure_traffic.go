// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/telekom/controlplane/rover-server/internal/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func mapExposureTraffic(in *roverv1.ApiExposure, out *api.ApiExposure) {
	if in.Traffic == nil {
		return
	}
	if in.Traffic.Failover != nil {
		out.Failover = api.Failover{
			Zones: in.Traffic.Failover.Zones,
		}
	}

	// Map rate limiting configuration
	mapRateLimit(in, out)
}

// mapRateLimit maps the rate limiting configuration from roverv1.ApiExposure to api.ApiExposure
func mapRateLimit(in *roverv1.ApiExposure, out *api.ApiExposure) {
	// If no rate limiting is configured, return without setting anything
	if in.Traffic == nil || in.Traffic.RateLimit == nil {
		return
	}

	// RateLimit container is already initialized in mapApiExposure

	// Map provider rate limits if configured
	if in.Traffic.RateLimit.Provider != nil {
		provider := api.RateLimit{}

		if in.Traffic.RateLimit.Provider.Limits != nil {
			provider.Second = int32(in.Traffic.RateLimit.Provider.Limits.Second)
			provider.Minute = int32(in.Traffic.RateLimit.Provider.Limits.Minute)
			provider.Hour = int32(in.Traffic.RateLimit.Provider.Limits.Hour)
		}

		// Map options
		provider.FaultTolerant = in.Traffic.RateLimit.Provider.Options.FaultTolerant
		provider.HideClientHeaders = in.Traffic.RateLimit.Provider.Options.HideClientHeaders

		out.RateLimit.Provider = provider
	}

	// Map consumer rate limits if configured
	if in.Traffic.RateLimit.Consumers != nil {
		// Map consumer default limits if configured
		if in.Traffic.RateLimit.Consumers.Default != nil {
			consumerDefault := api.RateLimit{}

			// Get limits from the Default struct
			consumerDefault.Second = int32(in.Traffic.RateLimit.Consumers.Default.Limits.Second)
			consumerDefault.Minute = int32(in.Traffic.RateLimit.Consumers.Default.Limits.Minute)
			consumerDefault.Hour = int32(in.Traffic.RateLimit.Consumers.Default.Limits.Hour)

			// ConsumerRateLimitDefaults doesn't have these fields in the API
			// Default to false for these values
			consumerDefault.FaultTolerant = false
			consumerDefault.HideClientHeaders = false

			out.RateLimit.ConsumerDefault = consumerDefault
		}

		// Map consumer overrides if configured
		if len(in.Traffic.RateLimit.Consumers.Overrides) > 0 {
			consumers := make([]api.ConsumerRateLimit, len(in.Traffic.RateLimit.Consumers.Overrides))

			for i, override := range in.Traffic.RateLimit.Consumers.Overrides {
				consumers[i].Id = override.Consumer

				// Get limits directly from the Limits field (which is a struct, not a pointer)
				consumers[i].Second = int32(override.Limits.Second)
				consumers[i].Minute = int32(override.Limits.Minute)
				consumers[i].Hour = int32(override.Limits.Hour)

				// ConsumerRateLimitOverrides doesn't have these fields in the API
				// Default to false for these values
				consumers[i].FaultTolerant = false
				consumers[i].HideClientHeaders = false
			}

			out.RateLimit.Consumers = consumers
		}
	}
}
