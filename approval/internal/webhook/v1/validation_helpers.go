// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// defaultDecisionFields fills in Timestamp and ResultingState on decisions
// that don't already have them set. This ensures every persisted decision
// carries an authoritative timestamp and the state it produced.
func defaultDecisionFields(decisions []approvalv1.Decision, state approvalv1.ApprovalState) {
	now := metav1.Now()
	for i := range decisions {
		if decisions[i].Timestamp == nil {
			decisions[i].Timestamp = &now
		}
		if decisions[i].ResultingState == "" {
			decisions[i].ResultingState = state
		}
	}
}

// validateDistinctDeciders checks that the last two decisions in the list were made by different people.
// This enforces the four-eyes principle: the same person cannot approve twice.
func validateDistinctDeciders(decisions []approvalv1.Decision) error {
	if len(decisions) < 2 {
		return apierrors.NewBadRequest(
			"FourEyes strategy requires at least two decisions for granting")
	}
	last := decisions[len(decisions)-1]
	secondLast := decisions[len(decisions)-2]

	lastEmail := strings.TrimSpace(last.Email)
	secondLastEmail := strings.TrimSpace(secondLast.Email)

	if lastEmail == "" || secondLastEmail == "" {
		return apierrors.NewBadRequest(
			"FourEyes strategy requires non-empty email addresses for the last two decisions")
	}

	if strings.EqualFold(lastEmail, secondLastEmail) {
		return apierrors.NewBadRequest(
			"FourEyes strategy requires two distinct deciders (by email)")
	}
	return nil
}
