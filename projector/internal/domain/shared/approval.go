// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"fmt"
	"strings"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
)

// ListenerActionPrefix starts the action of each Spectre Listener approval gate.
// A Listener has one gate per approval key, and Spectre sets the action to this
// prefix plus the key ("listen-provider", "listen-consumer").
const ListenerActionPrefix = "listen-"

// ValidateListenerGate returns an error unless key names a Listener approval
// gate and action is the action Spectre uses for that gate.
func ValidateListenerGate(key, action string) error {
	if key != approvalv1.ApprovalKeyProvider && key != approvalv1.ApprovalKeyConsumer {
		return fmt.Errorf("invalid Listener approvalKey %q: must be %q or %q", key, approvalv1.ApprovalKeyProvider, approvalv1.ApprovalKeyConsumer)
	}
	if action != ListenerActionPrefix+key {
		return fmt.Errorf("invalid Listener action %q: action must match approvalKey %q", action, key)
	}
	return nil
}

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
