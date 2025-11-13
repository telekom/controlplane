// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ConditionTypeApprovalGranted = "ApprovalGranted"
)

func newApprovalGrantedCondition(state approvalv1.ApprovalState, msg string) metav1.Condition {
	cond := metav1.Condition{
		Type:               ConditionTypeApprovalGranted,
		Status:             metav1.ConditionFalse,
		Reason:             string(state),
		Message:            msg,
		LastTransitionTime: metav1.Now(),
	}
	if state == approvalv1.ApprovalStateGranted {
		cond.Status = metav1.ConditionTrue
	}

	return cond
}
