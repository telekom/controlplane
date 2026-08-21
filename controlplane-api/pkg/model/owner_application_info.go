// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// OwnerApplicationInfo provides a reduced cross-tenant safe view of an owning application.
type OwnerApplicationInfo struct {
	ApplicationID int     `json:"applicationId"`
	IctoNumber    *string `json:"ictoNumber,omitempty"`
}
