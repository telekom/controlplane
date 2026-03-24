// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// ApiExposureInfo provides a reduced cross-tenant safe view of an API exposure.
// No navigable edges — traversal terminates here.
type ApiExposureInfo struct {
	ID                   int            `json:"id"`
	BasePath             string         `json:"basePath"`
	Visibility           string         `json:"visibility"`
	Active               bool           `json:"active"`
	ApiVersion           *string        `json:"apiVersion,omitempty"`
	Features             []string       `json:"features"`
	ApprovalConfig       ApprovalConfig `json:"approvalConfig"`
	OwnerApplicationName string         `json:"ownerApplicationName"`
	OwnerTeam            *TeamInfo      `json:"ownerTeam"`
}

// ApiSubscriptionInfo provides a reduced cross-tenant safe view of an API subscription.
// No navigable edges — traversal terminates here.
type ApiSubscriptionInfo struct {
	ID                   int       `json:"id"`
	BasePath             string    `json:"basePath"`
	StatusPhase          string    `json:"statusPhase"`
	StatusMessage        *string   `json:"statusMessage,omitempty"`
	OwnerApplicationName string    `json:"ownerApplicationName"`
	OwnerTeam            *TeamInfo `json:"ownerTeam"`
}
