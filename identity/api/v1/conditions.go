// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretRotationConditionType is the condition type used on Client resources to
// track the lifecycle of a graceful secret rotation. The Reason field
// distinguishes the phases: Accepted → Rotated → Completed.
var SecretRotationConditionType = "SecretRotation"

const (
	// SecretRotationReasonAccepted indicates that a secret change was detected
	// and the controller is about to trigger Keycloak's secret rotation.
	// This reason acts as a guard: on retry after a failed PUT, the handler
	// can detect that forceSecretRotation was already called for this
	// generation and skip it to avoid evicting the original secret from the
	// rotated slot.
	SecretRotationReasonAccepted = "Accepted"

	// SecretRotationReasonRotated indicates that the old secret has been moved
	// into Keycloak's rotated slot and the grace period is active.
	SecretRotationReasonRotated = "Rotated"

	// SecretRotationReasonCompleted indicates that the rotation cycle is
	// finished: the grace period expired and the old secret is no longer valid.
	SecretRotationReasonCompleted = "Completed"
)

// NewSecretRotationAcceptedCondition returns a condition indicating that a
// secret rotation has been accepted and forceSecretRotation is about to be
// called. Persisted by the controller on error, this condition prevents
// duplicate forceSecretRotation calls on retry.
func NewSecretRotationAcceptedCondition() metav1.Condition {
	return metav1.Condition{
		Type:               SecretRotationConditionType,
		Status:             metav1.ConditionTrue,
		Reason:             SecretRotationReasonAccepted,
		Message:            "Secret change detected, rotation accepted",
		LastTransitionTime: metav1.Now(),
	}
}

// NewSecretRotatedCondition returns a condition indicating that the old secret
// is in Keycloak's rotated slot with an active grace period.
func NewSecretRotatedCondition(rotatedAt time.Time) metav1.Condition {
	return metav1.Condition{
		Type:               SecretRotationConditionType,
		Status:             metav1.ConditionTrue,
		Reason:             SecretRotationReasonRotated,
		Message:            fmt.Sprintf("Client secret was rotated at %s", rotatedAt.UTC().Format(time.RFC3339)),
		LastTransitionTime: metav1.Now(),
	}
}

// NewSecretRotationCompletedCondition returns a condition indicating that the
// rotation cycle is finished and the grace period has expired.
func NewSecretRotationCompletedCondition() metav1.Condition {
	return metav1.Condition{
		Type:               SecretRotationConditionType,
		Status:             metav1.ConditionFalse,
		Reason:             SecretRotationReasonCompleted,
		Message:            "Rotation cycle completed, grace period expired",
		LastTransitionTime: metav1.Now(),
	}
}
