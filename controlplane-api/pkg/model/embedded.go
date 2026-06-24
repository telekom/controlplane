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

// EventScope defines a named scope with required trigger-based filtering for event exposure.
type EventScope struct {
	// Name is the unique identifier for this scope.
	Name string `json:"name"`

	// Trigger defines publisher-side filtering criteria for this scope.
	Trigger EventTrigger `json:"trigger"`
}

// EventTrigger defines filtering criteria for event delivery.
type EventTrigger struct {
	// ResponseFilter controls payload shaping (which fields to return).
	ResponseFilter *ResponseFilter `json:"responseFilter,omitempty"`

	// SelectionFilter controls event matching (which events to deliver).
	SelectionFilter *SelectionFilter `json:"selectionFilter,omitempty"`
}

// ResponseFilter controls which fields are included or excluded from the event payload.
type ResponseFilter struct {
	// Paths lists the JSON paths to include or exclude from the event payload.
	Paths []string `json:"paths,omitempty"`

	// Mode controls whether the listed paths are included or excluded.
	Mode string `json:"mode"` // "Include" or "Exclude"
}

// SelectionFilter defines criteria for selecting which events are delivered.
type SelectionFilter struct {
	// Attributes defines simple key-value equality matches on CloudEvents attributes.
	Attributes map[string]string `json:"attributes,omitempty"`

	// Expression contains an arbitrary JSON filter expression tree
	Expression string `json:"expression"`
}

// EventDelivery configures how events are delivered to the subscriber.
type EventDelivery struct {
	Payload                                   string `json:"payload"` // Data or DataRef
	EventRetentionTime                        string `json:"eventRetentionTime"`
	CircuitBreakerOptOut                      bool   `json:"circuitBreakerOptOut"`
	RetryableStatusCodes                      []int  `json:"retryableStatusCodes"`
	RedeliveriesPerSecond                     *int   `json:"redeliveriesPerSecond"`
	EnforceGetHttpRequestMethodForHealthCheck bool   `json:"enforceGetHttpRequestMethodForHealthCheck"`
}
