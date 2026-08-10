// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared

import "strings"

// MapApprovalState converts a PascalCase CR state to the SCREAMING_SNAKE ent enum.
func MapApprovalState(state string) string {
	return strings.ToUpper(state)
}

// MapApprovalStrategy converts a PascalCase CR strategy to the SCREAMING_SNAKE ent
// enum, handling the special FourEyes -> FOUR_EYES case.
func MapApprovalStrategy(strategy string) string {
	switch strategy {
	case "Auto":
		return "AUTO"
	case "Simple":
		return "SIMPLE"
	case "FourEyes":
		return "FOUR_EYES"
	default:
		return strings.ToUpper(strategy)
	}
}
