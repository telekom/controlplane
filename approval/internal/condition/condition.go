// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	approved = metav1.Condition{
		Type:    "Approved",
		Status:  metav1.ConditionTrue,
		Reason:  "Granted",
		Message: "Request has been granted",
	}
)

func NewApprovedCondition() metav1.Condition {
	return approved
}

func NewSuspendedCondition() metav1.Condition {
	return metav1.Condition{
		Type:    "Approved",
		Status:  metav1.ConditionTrue,
		Reason:  "Suspended",
		Message: "Request has been suspended",
	}
}

func NewRejectedCondition() metav1.Condition {
	return metav1.Condition{
		Type:    "Approved",
		Status:  metav1.ConditionFalse,
		Reason:  "Rejected",
		Message: "Request has been rejected",
	}
}

func NewPendingCondition() metav1.Condition {
	return metav1.Condition{
		Type:    "Approved",
		Status:  metav1.ConditionFalse,
		Reason:  "Pending",
		Message: "Request is pending",
	}
}
