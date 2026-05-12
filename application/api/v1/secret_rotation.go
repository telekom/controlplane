// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

const (
	// SecretRotationConditionType is the condition type used to track secret rotation state on the Application CR.
	SecretRotationConditionType = "SecretRotation"
	// SecretRotationReasonInProgress indicates a rotation has been initiated but not yet fully propagated.
	SecretRotationReasonInProgress = "InProgress"
	// SecretRotationReasonSuccess indicates a rotation has been fully propagated to all sub-resources.
	SecretRotationReasonSuccess = "Success"

	// AnnotationGracefulRotation is set to "true" on the Application when a graceful
	// secret rotation (with grace period) was initiated by the webhook.
	AnnotationGracefulRotation = "application.cp.ei.telekom.de/graceful-secret-rotation"
)
