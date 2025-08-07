// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"slices"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

type Traffic struct {
	// Failover defines the failover configuration for the API exposure.
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
	// RateLimit defines request rate limiting for this API
	// +kubebuilder:validation:Optional
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

type SubscriberTraffic struct {
	// Failover defines the failover configuration for the API exposure.
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
}

type Failover struct {
	// Zone is the zone to which the traffic should be failed over in case of an error.
	// +kubebuilder:validation:Required
	Zones []ctypes.ObjectRef `json:"zone"`
}

// -------
// Rate Limiting Section
// -------

// RateLimit defines rate limiting configuration for an API
type RateLimit struct {
	// Provider defines request rate limiting for this API
	// +kubebuilder:validation:Optional
	Provider *RateLimitConfig `json:"provider,omitempty"`
	// SubscriberRateLimit defines request rate limiting for this API per subscriber
	// +kubebuilder:validation:Optional
	SubscriberRateLimit *SubscriberRateLimits `json:"subscriberRateLimit,omitempty"`
}

// RateLimitConfig defines rate limits for different time windows
type RateLimitConfig struct {
	Limits  Limits           `json:"limits"`
	Options RateLimitOptions `json:"options,omitempty"`
}

// Limits defines the actual rate limit values for different time windows
type Limits struct {
	// Second defines the maximum number of requests allowed per second
	// +kubebuilder:validation:Minimum=0
	Second int `json:"second,omitempty"`
	// Minute defines the maximum number of requests allowed per minute
	// +kubebuilder:validation:Minimum=0
	Minute int `json:"minute,omitempty"`
	// Hour defines the maximum number of requests allowed per hour
	// +kubebuilder:validation:Minimum=0
	Hour int `json:"hour,omitempty"`
}

// RateLimitOptions defines additional configuration options for rate limiting
type RateLimitOptions struct {
	// HideClientHeaders hides additional client headers which give information about the rate-limit, reset and remaining requests for consumers if set to true.
	// +kubebuilder:default=false
	HideClientHeaders *bool `json:"hideClientHeaders,omitempty"`
	// FaultTolerant defines if the rate limit plugin should be fault tolerant, if gateway is not able to access the config store
	// +kubebuilder:default=true
	FaultTolerant *bool `json:"faultTolerant,omitempty"`
}

// SubscriberRateLimits defines rate limits for API subscribers
type SubscriberRateLimits struct {
	// Default defines the rate limit applied to all consumers not specifically overridden
	// +kubebuilder:validation:Optional
	Default *SubscriberRateLimitDefaults `json:"default,omitempty"`
	// Overrides defines consumer-specific rate limits, keyed by consumer identifier
	// +kubebuilder:validation:MaxItems=10
	Overrides []RateLimitOverrides `json:"overrides,omitempty"`
}

type SubscriberRateLimitDefaults struct {
	// +kubebuilder:validation:Required
	Limits Limits `json:"limits"`
}

type RateLimitOverrides struct {
	// Subscriber is the unique identifier of the subscriber
	// +kubebuilder:validation:MinLength=1
	Subscriber string `json:"subscriber"`
	Limits     Limits `json:"limits"`
}

func (t *Traffic) HasFailover() bool {
	return t.Failover != nil && len(t.Failover.Zones) > 0
}

func (t *Traffic) HasRateLimit() bool {
	return t.RateLimit != nil
}

func (t *Traffic) HasSubscriberRateLimit() bool {
	if !t.HasRateLimit() {
		return false
	}
	return t.RateLimit.SubscriberRateLimit != nil
}

func (f Failover) ContainsZone(zone ctypes.ObjectRef) bool {
	return slices.ContainsFunc(f.Zones, func(z ctypes.ObjectRef) bool {
		return z.Equals(&zone)
	})
}
