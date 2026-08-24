// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// OwnerApplicationInfo provides a reduced cross-tenant safe view of an owning application.
type OwnerApplicationInfo struct {
	ApplicationID int                          `json:"applicationId"`
	ExternalIDs   []OwnerApplicationExternalID `json:"externalIds"`
}

// OwnerApplicationExternalID represents an external identifier and its scheme.
type OwnerApplicationExternalID struct {
	ID     string `json:"id"`
	Scheme string `json:"scheme"`
}
