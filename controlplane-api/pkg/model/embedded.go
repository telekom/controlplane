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
