// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewApiCondition creates a condition indicating whether the corresponding API exists and is active.
func NewApiCondition(apiExp *apiv1.ApiExposure, found bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "ApiExists",
		Status:             metav1.ConditionFalse,
		Reason:             "NoApi",
		Message:            "Corresponding API does not exist or is not active",
		LastTransitionTime: metav1.Now(),
	}
	if found {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "ApiExists"
		cond.Message = "Corresponding API exists and is active"
	}
	return cond
}

// NewApiExposureActiveCondition creates a condition indicating whether the ApiExposure is active.
func NewApiExposureActiveCondition(apiExp *apiv1.ApiExposure, active bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "ApiExposureActive",
		Status:             metav1.ConditionFalse,
		Reason:             "NotActive",
		Message:            "API is already exposed by another ApiExposure",
		LastTransitionTime: metav1.Now(),
	}
	if active {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "Active"
		cond.Message = "API is exposed by this ApiExposure"
	}
	return cond
}
