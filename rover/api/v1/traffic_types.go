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
	// CircuitBreaker defines the Kong circuit breaker configuration
	// +kubebuilder:validation:Optional
	CircuitBreaker *CircuitBreaker `json:"circuitBreaker,omitempty"`
}

type CircuitBreaker struct {
	// CircuitBreaker flags if the Kong circuit breaker feature should be used
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
}

type SubscriberTraffic struct {
	// Failover defines disaster recovery configuration for this API
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
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

// -------
// Rate Limiting Section
// -------

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
	// +kubebuilder:validation:Optional
	Limits *Limits `json:"limits,omitempty"`
	// +kubebuilder:validation:Optional
	Options RateLimitOptions `json:"options,omitempty"`
}

// Limits defines the actual rate limit values for different time windows
type Limits struct {
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

// RateLimitOptions defines additional configuration options for rate limiting
type RateLimitOptions struct {
	// HideClientHeaders hides additional client headers which give information about the rate-limit, reset and remaining requests for consumers if set to true.
	// +kubebuilder:default=false
	HideClientHeaders bool `json:"hideClientHeaders,omitempty"`
	// FaultTolerant defines if the rate limit plugin should be fault tolerant, if gateway is not able to access the config store
	// +kubebuilder:default=true
	FaultTolerant bool `json:"faultTolerant,omitempty"`
}

// ConsumerRateLimits defines rate limits for API consumers on a per-consumer basis and a default config
type ConsumerRateLimits struct {
	// Default defines the rate limit applied to all consumers not specifically overridden
	// +kubebuilder:validation:Optional
	Default *ConsumerRateLimitDefaults `json:"default,omitempty"`
	// Overrides defines consumer-specific rate limits
	// +kubebuilder:validation:MaxItems=10
	Overrides []ConsumerRateLimitOverrides `json:"overrides,omitempty"`
}

type ConsumerRateLimitDefaults struct {
	// +kubebuilder:validation:Required
	Limits Limits `json:"limits"`
}

type ConsumerRateLimitOverrides struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Consumer string `json:"consumer"`
	// +kubebuilder:validation:Required
	Limits Limits `json:"limits"`
}

func (t *Traffic) HasFailover() bool {
	return t.Failover != nil && len(t.Failover.Zones) > 0
}

func (t *Traffic) HasRateLimit() bool {
	return t.RateLimit != nil
}

func (t *Traffic) HasProviderRateLimit() bool {
	if !t.HasRateLimit() {
		return false
	}
	return t.RateLimit.Provider != nil
}

func (t *Traffic) HasProviderRateLimitLimits() bool {
	if !t.HasProviderRateLimit() {
		return false
	}
	return t.RateLimit.Provider.Limits != nil
}

func (t *Traffic) HasConsumerRateLimit() bool {
	if !t.HasRateLimit() {
		return false
	}
	return t.RateLimit.Consumers != nil
}

func (t *Traffic) HasConsumerDefaultsRateLimit() bool {
	if !t.HasConsumerRateLimit() {
		return false
	}
	return t.RateLimit.Consumers.Default != nil
}

func (t *Traffic) HasConsumerOverridesRateLimit() bool {
	if !t.HasConsumerRateLimit() {
		return false
	}
	return t.RateLimit.Consumers.Overrides != nil
}

func (t *Traffic) HasCircuitBreaker() bool {
	return t.CircuitBreaker != nil
}
