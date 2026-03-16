// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// Member represents a team member.
type Member struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

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

// ApiInfo represents the base API definition with version and category information.
type ApiInfo struct {
	BasePath string `json:"basePath"`
	Version  string `json:"version"`
	Category string `json:"category"`
	Active   bool   `json:"active"`
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

// ResourceStatus represents the simplified resource status derived from K8s conditions.
type ResourceStatus struct {
	Phase   ResourceStatusPhase `json:"phase"`
	Message *string             `json:"message,omitempty"`
}

// ResourceStatusPhase represents the high-level phase of a resource.
type ResourceStatusPhase string

const (
	ResourceStatusPhaseReady   ResourceStatusPhase = "READY"
	ResourceStatusPhasePending ResourceStatusPhase = "PENDING"
	ResourceStatusPhaseError   ResourceStatusPhase = "ERROR"
	ResourceStatusPhaseUnknown ResourceStatusPhase = "UNKNOWN"
)
