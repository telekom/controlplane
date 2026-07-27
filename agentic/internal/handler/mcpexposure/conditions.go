// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpexposure

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewMcpServerCondition creates a condition indicating whether the corresponding McpServer
// exists and is active for this McpExposure.
func NewMcpServerCondition(found bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "McpServerExists",
		Status:             metav1.ConditionFalse,
		Reason:             "NoMcpServer",
		Message:            "Corresponding McpServer does not exist or is not active",
		LastTransitionTime: metav1.Now(),
	}
	if found {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "McpServerExists"
		cond.Message = "Corresponding McpServer exists and is active"
	}
	return cond
}

// NewMcpExposureActiveCondition creates a condition indicating whether this McpExposure
// is the active one for its basePath (oldest-wins).
func NewMcpExposureActiveCondition(active bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "McpExposureActive",
		Status:             metav1.ConditionFalse,
		Reason:             "NotActive",
		Message:            "BasePath is already exposed by another McpExposure",
		LastTransitionTime: metav1.Now(),
	}
	if active {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "Active"
		cond.Message = "BasePath is exposed by this McpExposure"
	}
	return cond
}
