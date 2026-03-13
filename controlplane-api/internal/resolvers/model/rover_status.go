// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// RoverStatus provides an aggregated status overview from the Rover perspective.
type RoverStatus struct {
	Phase               ResourceStatusPhase `json:"phase"`
	TotalExposures      int                 `json:"totalExposures"`
	TotalSubscriptions  int                 `json:"totalSubscriptions"`
	ActiveExposures     int                 `json:"activeExposures"`
	ActiveSubscriptions int                 `json:"activeSubscriptions"`
}
