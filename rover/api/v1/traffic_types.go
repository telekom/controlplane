// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// Traffic defines traffic management configuration for an API
type Traffic struct {
	// LoadBalancing defines how traffic is distributed among multiple upstream servers
	// +kubebuilder:validation:Optional
	LoadBalancing *LoadBalancing `json:"loadBalancing,omitempty"`
	// Failover defines disaster recovery configuration for this API
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
	// RateLimit defines request rate limiting for this API
	// +kubebuilder:validation:Optional
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

type SubscriberTraffic struct {
	// Failover defines disaster recovery configuration for this API
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
	// RateLimit defines request rate limiting for this API
	// +kubebuilder:validation:Optional
	RateLimit *RateLimitConfig `json:"rateLimit,omitempty"`
}

// LoadBalancing defines load balancing strategy for multiple upstreams
type LoadBalancing struct {
	// Strategy defines the algorithm used for distributing traffic (RoundRobin, LeastConnections)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=RoundRobin;LeastConnections
	// +kubebuilder:default=RoundRobin
	Strategy LoadBalancingStrategy `json:"strategy,omitempty"`
}

// LoadBalancingStrategy defines how traffic is distributed among multiple upstreams
type LoadBalancingStrategy string

const (
	// LoadBalancingRoundRobin distributes requests evenly across all upstreams
	LoadBalancingRoundRobin LoadBalancingStrategy = "RoundRobin"
	// LoadBalancingLeastConnections sends requests to the upstream with the fewest active connections
	LoadBalancingLeastConnections LoadBalancingStrategy = "LeastConnections"
)

// Failover defines failover configuration for disaster recovery
type Failover struct {
	// Zones is a list of zone names to use for failover if the primary zone is unavailable
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=10
	Zones []string `json:"zones,omitempty"`
}

// RateLimit defines rate limiting configuration for an API
type RateLimit struct {
	// Provider defines rate limits applied by the API provider (owner)
	// +kubebuilder:validation:Optional
	Provider *RateLimitConfig `json:"provider,omitempty"`
	// Consumers defines rate limits applied to API consumers (clients)
	// +kubebuilder:validation:Optional
	Consumers *ConsumerRateLimits `json:"consumers,omitempty"`
}

// RateLimitConfig defines rate limits for different time windows
type RateLimitConfig struct {
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

// ConsumerRateLimits defines rate limits for API consumers
type ConsumerRateLimits struct {
	// Default defines the rate limit applied to all consumers not specifically overridden
	// +kubebuilder:validation:Optional
	Default *RateLimitConfig `json:"default,omitempty"`
	// Overrides defines consumer-specific rate limits, keyed by consumer identifier
	// +kubebuilder:validation:Optional
	Overrides map[string]RateLimitConfig `json:"overrides,omitempty"`
}
