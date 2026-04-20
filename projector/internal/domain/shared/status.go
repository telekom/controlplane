// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatusFromConditions extracts the ent status_phase and status_message
// from the CR's conditions by reading the latest "Ready" condition.
//
// Mapping:
//
//	Ready=True                       -> "READY"
//	Ready=False, Reason=Error|Failed -> "ERROR"
//	Ready=False, other reason        -> "PENDING"
//	Ready condition missing          -> "UNKNOWN"
func StatusFromConditions(conditions []metav1.Condition) (phase, message string) {
	ready := meta.FindStatusCondition(conditions, "Ready")
	if ready == nil {
		return "UNKNOWN", ""
	}
	switch ready.Status {
	case metav1.ConditionTrue:
		return "READY", ready.Message
	case metav1.ConditionFalse:
		if ready.Reason == "Error" || ready.Reason == "Failed" {
			return "ERROR", ready.Message
		}
		return "PENDING", ready.Message
	default:
		return "UNKNOWN", ready.Message
	}
}
