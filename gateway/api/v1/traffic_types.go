// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

type Traffic struct {
	Failover  *Failover  `json:"failover,omitempty"`
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

type ConsumeRouteTraffic struct {
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

type Failover struct {
	TargetZoneName string     `json:"targetZoneName"`
	Upstreams      []Upstream `json:"upstreams"`
	Security       *Security  `json:"security,omitempty"`
}

// RateLimit defines rate limits for different time windows
type RateLimit struct {
	// +kubebuilder:validation:Required
	Limits Limits `json:"limits"`
	// +kubebuilder:validation:Optional
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
