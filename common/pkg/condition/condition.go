// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeProcessing = "Processing"
	ConditionTypeReady      = "Ready"
)

// Processing condition reasons.
const (
	ReasonProcessing = "Processing"
	ReasonBlocked    = "Blocked"
	ReasonDone       = "Done"
)

// Ready condition reasons — Success.
const (
	// ReasonProvisioned indicates all sub-resources have been successfully provisioned.
	ReasonProvisioned = "Provisioned"
)

// Ready condition reasons — Processing (transient, will resolve on its own).
const (
	// ReasonSubResourceNotReady indicates at least one child/sub-resource is not yet ready.
	ReasonSubResourceNotReady = "SubResourceNotReady"
	// ReasonProvisioning indicates the resource is actively being set up.
	ReasonProvisioning = "Provisioning"
)

// Ready condition reasons — Blocked (user-actionable, cannot progress without intervention).
const (
	// ReasonPreconditionNotMet indicates a required precondition is not satisfied.
	ReasonPreconditionNotMet = "PreconditionNotMet"
	// ReasonApprovalPending indicates the resource is awaiting approval.
	ReasonApprovalPending = "ApprovalPending"
	// ReasonAccessDenied indicates the caller lacks required permissions.
	ReasonAccessDenied = "AccessDenied"
	// ReasonValidationFailed indicates the resource spec failed validation.
	ReasonValidationFailed = "ValidationFailed"
)

// Ready condition reasons — Error (internal, not user-controllable).
const (
	// ReasonError indicates an internal error occurred that the user cannot resolve.
	ReasonError = "Error"
)

var (
	ProcessingCondition = metav1.Condition{
		Type:   ConditionTypeProcessing,
		Status: metav1.ConditionTrue,
	}

	ReadyCondition = metav1.Condition{
		Type:   ConditionTypeReady,
		Status: metav1.ConditionTrue,
	}
)

func NewBlockedCondition(message string) metav1.Condition {
	condition := ProcessingCondition
	condition.Status = metav1.ConditionFalse
	condition.Message = message
	condition.Reason = ReasonBlocked
	return condition
}

func NewProcessingCondition(reason, message string) metav1.Condition {
	condition := ProcessingCondition
	condition.Message = message
	condition.Reason = reason
	return condition
}

func NewDoneProcessingCondition(message string) metav1.Condition {
	condition := ProcessingCondition
	condition.Status = metav1.ConditionFalse
	condition.Message = message
	condition.Reason = ReasonDone
	return condition
}

func NewReadyCondition(reason, message string) metav1.Condition {
	condition := ReadyCondition
	condition.Message = message
	condition.Reason = reason
	return condition
}

func NewNotReadyCondition(reason, message string) metav1.Condition {
	condition := ReadyCondition
	condition.Status = metav1.ConditionFalse
	condition.Reason = reason
	condition.Message = message
	return condition
}

func SetToUnknown(cond metav1.Condition) metav1.Condition { //nolint:gocritic // hugeParam: intentional value copy for functional transformation
	cond.Status = metav1.ConditionUnknown
	cond.Reason = "Unknown"
	cond.Message = ""
	return cond
}
