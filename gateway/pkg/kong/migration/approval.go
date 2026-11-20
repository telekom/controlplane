// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Mode is the Migration Mode of the gateway operator.
type Mode string

const (
	// ModeOff writes to Kong without comparing.
	ModeOff Mode = "off"
	// ModeOn blocks a change with a diff that is neither ignored nor approved.
	ModeOn Mode = "on"
	// ModeStrict blocks every change that is not approved, even one whose diffs are all ignored.
	ModeStrict Mode = "strict"
)

// ParseMode parses the value of the --migration-mode flag.
func ParseMode(value string) (Mode, error) {
	switch mode := Mode(value); mode {
	case ModeOff, ModeOn, ModeStrict:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid migration mode %q, expected %q, %q or %q", value, ModeOff, ModeOn, ModeStrict)
	}
}

// ApprovalAnnotation approves the pending Kong change of a Route or Consumer. Its value is
// the hash of the change from status.migration.hash, so it approves exactly the reviewed
// change: any other change, such as a later edit of Kong by the legacy operator, has
// another hash and is blocked again.
const ApprovalAnnotation = "gateway.cp.ei.telekom.de/migration-approved-hash"

// IsApproved reports whether obj approves the change with hash.
func IsApproved(obj metav1.Object, hash string) bool {
	return hash != "" && obj.GetAnnotations()[ApprovalAnnotation] == hash
}
