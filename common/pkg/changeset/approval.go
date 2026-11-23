// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/telekom/controlplane/common/pkg/config"
)

// Mode is the Migration Mode of an operator.
type Mode string

const (
	// ModeOff writes every change. The changes are still planned and recorded.
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

// ApprovalAnnotation approves the pending backend change of an object. Its value is the
// hash of the change from status.migration.hash, so it approves exactly the reviewed
// change: any other change, such as a later edit of the backend by the legacy operator,
// has another hash and is blocked again.
const ApprovalAnnotation = config.LabelKeyPrefix + "/migration-approved-hash"

// IsApproved reports whether obj approves the change with hash.
func IsApproved(obj metav1.Object, hash string) bool {
	return hash != "" && obj.GetAnnotations()[ApprovalAnnotation] == hash
}

// ApprovalChangedPredicate admits an update that changes the approval annotation. The
// annotation does not change the generation, so without it an approval would only take
// effect on the next periodic reconcile.
func ApprovalChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectOld.GetAnnotations()[ApprovalAnnotation] != e.ObjectNew.GetAnnotations()[ApprovalAnnotation]
		},
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}
