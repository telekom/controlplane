// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewServerCondition creates a condition indicating whether the corresponding server
// (McpServer or AgentCard) exists and is active for this AgenticExposure.
func NewServerCondition(found bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "ServerExists",
		Status:             metav1.ConditionFalse,
		Reason:             "ServerNotFound",
		Message:            "Corresponding agentic server (McpServer, AgentCard) does not exist or is not active",
		LastTransitionTime: metav1.Now(),
	}
	if found {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "ServerExists"
		cond.Message = "Corresponding server exists and is active"
	}
	return cond
}

// NewAgenticExposureActiveCondition creates a condition indicating whether this AgenticExposure
// is the active one for its basePath (oldest-wins).
func NewAgenticExposureActiveCondition(active bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "AgenticExposureActive",
		Status:             metav1.ConditionFalse,
		Reason:             "NotActive",
		Message:            "BasePath is already exposed by another AgenticExposure",
		LastTransitionTime: metav1.Now(),
	}
	if active {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "Active"
		cond.Message = "BasePath is exposed by this AgenticExposure"
	}
	return cond
}
