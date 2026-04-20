// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// TeamInfo provides a reduced cross-tenant safe view of a team.
type TeamInfo struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	GroupName string  `json:"groupName"`
	Email     *string `json:"email,omitempty"`
}
