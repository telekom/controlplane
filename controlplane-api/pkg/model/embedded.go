// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package model provides shared domain types used by ent schemas and consumed
// by external modules (e.g. projector). These types were extracted from the
// internal resolvers package to make them importable across module boundaries.
package model

// Upstream represents an upstream service endpoint.
type Upstream struct {
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

// ApprovalConfig represents the approval workflow configuration on an exposure.
type ApprovalConfig struct {
	Strategy     string   `json:"strategy"`
	TrustedTeams []string `json:"trustedTeams"`
}

// RequesterInfo represents who requested an approval.
type RequesterInfo struct {
	TeamName        string  `json:"teamName"`
	TeamEmail       string  `json:"teamEmail"`
	Reason          *string `json:"reason,omitempty"`
	ApplicationName *string `json:"applicationName,omitempty"`
}

// DeciderInfo represents who decides on an approval.
type DeciderInfo struct {
	TeamName  string  `json:"teamName"`
	TeamEmail *string `json:"teamEmail,omitempty"`
}

// Decision represents a decision made on an approval.
type Decision struct {
	Name           string  `json:"name"`
	Email          *string `json:"email,omitempty"`
	Comment        *string `json:"comment,omitempty"`
	Timestamp      *string `json:"timestamp,omitempty"`
	ResultingState *string `json:"resultingState,omitempty"`
}

// AvailableTransition represents a valid state transition from the current state.
type AvailableTransition struct {
	Action  string `json:"action"`
	ToState string `json:"toState"`
}

// IpRestrictions represents the IP allowlist and denylist for an application.
type IpRestrictions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// ExternalId represents an external identifier for an application.
type ExternalId struct {
	Id     string `json:"id"`
	Scheme string `json:"scheme"`
}

// Security

// ApiExposureSecurity represents the security config
type ApiExposureSecurity struct {
	M2M *Machine2MachineAuthentication `json:"m2m,omitempty"`
}

// ApiSubscriptionSecurity defines the security configuration for the Rover
type ApiSubscriptionSecurity struct {
	M2M *SubscriberMachine2MachineAuthentication `json:"m2m,omitempty"`
}

// ApiExposureMachine2MachineAuthentication for Machine2Machine communication
type Machine2MachineAuthentication struct {
	ExternalIDP *ExternalIdentityProvider `json:"externalIDP,omitempty"` // optional/nillable
	Basic       *BasicAuthCredentials     `json:"basic,omitempty"`       // optional/nillable
	Scopes      []string                  `json:"scopes,omitempty"`
}

// SubscriberMachine2MachineAuthentication defines the authentication methods for machine-to-machine communication for subscribers
type SubscriberMachine2MachineAuthentication struct {
	Client *OAuth2ClientCredentials `json:"client,omitempty"` // optional/nillable
	Basic  *BasicAuthCredentials    `json:"basic,omitempty"`  // optional/nillable
	Scopes []string                 `json:"scopes,omitempty"` // optional/nillable
}

// ExternalIdentityProvider defines configuration for using an external identity provider
type ExternalIdentityProvider struct {
	TokenEndpoint string                   `json:"tokenEndpoint"`          // optional/nillable
	TokenRequest  *string                  `json:"tokenRequest,omitempty"` // optional/nillable - only "client_secret_basic" or "client_secret_post"
	GrantType     *string                  `json:"grantType,omitempty"`    // optional/nillable
	Basic         *BasicAuthCredentials    `json:"basic,omitempty"`        // optional/nillable
	Client        *OAuth2ClientCredentials `json:"client,omitempty"`       // optional/nillable
}

// BasicAuthCredentials defines username/password credentials
type BasicAuthCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// OAuth2ClientCredentials defines client credentials for OAuth2
type OAuth2ClientCredentials struct {
	ClientId     string  `json:"clientId"`
	ClientSecret *string `json:"clientSecret,omitempty"` // optional/nillable
	ClientKey    *string `json:"clientKey,omitempty"`    // optional/nillable
}

// Traffic
type Traffic struct {
	Failover  *Failover  `json:"failover,omitempty"`
	RateLimit *RateLimit `json:"rateLimit,omitempty"` // optional/nillable
}

// RateLimit defines the exporure based rate limiting
type RateLimit struct {
	Provider            *RateLimitConfig      `json:"provider,omitempty"`            // optional
	SubscriberRateLimit *SubscriberRateLimits `json:"subscriberRateLimit,omitempty"` // optional
}

type Failover struct {
	// Zones reflect only the zones as string, no cross-tenant references, as the consumer based
	// failover is deprecated and is subject to be removed
	Zones []string `json:"zones,omitempty"`
}

// RateLimitConfig defines rate limits for different time windows
type RateLimitConfig struct {
	Limits  Limits           `json:"limits"`
	Options RateLimitOptions `json:"options,omitempty"` // optional
}

// Limits defines the actual rate limit values for different time windows
type Limits struct {
	Second int `json:"second,omitempty"` // optional
	Minute int `json:"minute,omitempty"` // optional
	Hour   int `json:"hour,omitempty"`   // optional
}

// RateLimitOptions defines additional configuration options for rate limiting
type RateLimitOptions struct {
	HideClientHeaders bool `json:"hideClientHeaders,omitempty"` // optional
	FaultTolerant     bool `json:"faultTolerant,omitempty"`     // optional
}

// SubscriberRateLimits defines rate limits for API subscribers
type SubscriberRateLimits struct {
	Default   *SubscriberRateLimitDefaults `json:"default,omitempty"`
	Overrides []RateLimitOverrides         `json:"overrides,omitempty"`
}

type SubscriberRateLimitDefaults struct {
	Limits Limits `json:"limits"`
}

type RateLimitOverrides struct {
	Subscriber string `json:"subscriber"`
	Limits     Limits `json:"limits"`
}
