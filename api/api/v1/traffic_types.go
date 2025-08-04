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
	// SubscriberRateLimit defines request rate limiting for this API per subscriber
	// +kubebuilder:validation:Optional
	SubscriberRateLimit *SubscriberRateLimit `json:"subscriberRateLimit,omitempty"`
}

type SubscriberTraffic struct {
	// Failover defines the failover configuration for the API exposure.
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
	// RateLimit defines request rate limiting for this API
	// +kubebuilder:validation:Optional
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

type Failover struct {
	// Zone is the zone to which the traffic should be failed over in case of an error.
	// +kubebuilder:validation:Required
	Zones []ctypes.ObjectRef `json:"zone"`
}

// SubscriberRateLimit defines rate limits for API consumers
type SubscriberRateLimit struct {
	// Default defines the rate limit applied to all consumers not specifically overridden
	// +kubebuilder:validation:Optional
	Default *RateLimit `json:"default,omitempty"`
	// Overrides defines consumer-specific rate limits, keyed by consumer identifier
	// +kubebuilder:validation:Optional
	Overrides []RateLimitOverrides `json:"overrides,omitempty"`
}

type RateLimitOverrides struct {
	Consumer  string `json:"consumer"`
	RateLimit `json:",inline"`
}

// RateLimit defines rate limits for different time windows
type RateLimit struct {
	Limits LimitConfig `json:"limits"`
	// +kubebuilder:validation:Optional
	Options LimitOptions `json:"options,omitempty"`
}

// LimitConfig defines the actual rate limit values for different time windows
// +kubebuilder:validation:XValidation:rule="self.second < self.minute || self.second == 0 || self.minute == 0",message="Second must be less than minute"
// +kubebuilder:validation:XValidation:rule="self.minute < self.hour || self.minute == 0 || self.hour == 0",message="Minute must be less than hour"
// +kubebuilder:validation:XValidation:rule="self.second != 0 || self.minute != 0 || self.hour != 0",message="At least one of second, minute, or hour must be specified"
type LimitConfig struct {
	// Second defines the maximum number of requests allowed per second
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	Second int `json:"second,omitempty"`
	// Minute defines the maximum number of requests allowed per minute
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	Minute int `json:"minute,omitempty"`
	// Hour defines the maximum number of requests allowed per hour
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	Hour int `json:"hour,omitempty"`
}

// LimitOptions defines additional configuration options for rate limiting
type LimitOptions struct {
	// HideClientHeaders hides additional client headers which give information about the rate-limit, reset and remaining requests for consumers if set to true.
	HideClientHeaders bool `json:"hideClientHeaders,omitempty"`
	// FaultTolerant defines if the rate limit plugin should be fault tolerant, if gateway is not able to access the config store
	FaultTolerant bool `json:"faultTolerant,omitempty"`
}

func (t *Traffic) HasFailover() bool {
	return t.Failover != nil && len(t.Failover.Zones) > 0
}

func (t *Traffic) HasRateLimit() bool {
	return t.RateLimit != nil
}

func (t *Traffic) HasSubscriberRateLimit() bool {
	return t.SubscriberRateLimit != nil
}

func (f Failover) ContainsZone(zone ctypes.ObjectRef) bool {
	return slices.ContainsFunc(f.Zones, func(z ctypes.ObjectRef) bool {
		return z.Equals(&zone)
	})
}
