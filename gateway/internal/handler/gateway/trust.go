// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

const (
	// ConditionTypeTrustReady reports whether provider CA trust was resolved.
	ConditionTypeTrustReady = "TrustReady"
	// ReasonTrustResolved is set when trust resolution succeeded.
	ReasonTrustResolved = "TrustResolved"
)

// SetTrustCondition sets TrustReady from the result of a client acquisition or validation.
// Errors unrelated to trust leave the condition unchanged.
func SetTrustCondition(obj ctypes.Object, err error) {
	if err == nil {
		obj.SetCondition(metav1.Condition{
			Type:    ConditionTypeTrustReady,
			Status:  metav1.ConditionTrue,
			Reason:  ReasonTrustResolved,
			Message: "Provider trust resolved",
		})
		return
	}
	if te := kongutil.AsTrustError(err); te != nil {
		obj.SetCondition(metav1.Condition{
			Type:    ConditionTypeTrustReady,
			Status:  metav1.ConditionFalse,
			Reason:  te.Reason,
			Message: te.Error(),
		})
	}
}
