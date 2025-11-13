// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"fmt"
	"strings"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewApiCondition creates a condition indicating whether the corresponding API exists and is active.
func NewApiCondition(apiSub *apiv1.ApiSubscription, found bool) metav1.Condition {
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

// NewApiExposureCondition creates a condition indicating whether the corresponding ApiExposure exists and is active.
func NewApiExposureCondition(apiSub *apiv1.ApiSubscription, found bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "ApiExposureExists",
		Status:             metav1.ConditionFalse,
		Reason:             "NoApiExposure",
		Message:            "Corresponding ApiExposure does not exist or is not active",
		LastTransitionTime: metav1.Now(),
	}
	if found {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "ApiExposureExists"
		cond.Message = "Corresponding ApiExposure exists and is active"
	}
	return cond
}

// NewVisibilityAllowedCondition creates a condition indicating whether the ApiSubscription visibility is allowed by the ApiExposure.
func NewVisibilityAllowedCondition(apiSub *apiv1.ApiSubscription, visibility string, allowed bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "VisibilityAllowed",
		Status:             metav1.ConditionFalse,
		Reason:             "NotAllowed",
		Message:            fmt.Sprintf("ApiSubscription visibility %q is not allowed by the ApiExposure", visibility),
		LastTransitionTime: metav1.Now(),
	}
	if allowed {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "Allowed"
		cond.Message = fmt.Sprintf("ApiSubscription visibility %q is allowed by the ApiExposure", visibility)
	}
	return cond
}

// NewScopesAllowedCondition creates a condition indicating whether the ApiSubscription scopes are allowed by the ApiExposure.
func NewScopesAllowedCondition(apiSub *apiv1.ApiSubscription, scopes []string, allowed bool) metav1.Condition {
	cond := metav1.Condition{
		Type:               "ScopesAllowed",
		Status:             metav1.ConditionFalse,
		Reason:             "NotAllowed",
		Message:            "ApiSubscription scopes are not allowed by the ApiExposure",
		LastTransitionTime: metav1.Now(),
	}
	if !allowed && len(scopes) > 0 {
		cond.Message = "ApiSubscription scopes are not allowed by the ApiExposure: " + strings.Join(scopes, ", ")
	}
	if allowed && len(scopes) > 0 {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "Allowed"
		cond.Message = "ApiSubscription scopes are allowed by the ApiExposure: " + strings.Join(scopes, ", ")
	} else if allowed {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "Allowed"
		cond.Message = "ApiSubscription has no scopes defined, so they are allowed by default"
	}
	return cond
}
