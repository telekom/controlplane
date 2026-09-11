// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ConditionTypeApprovalGranted = "ApprovalGranted"
)

// Ready condition reasons set by consumers of the approval builder.
const (
	// ReasonApprovalPending indicates the resource is waiting for an approval decision.
	ReasonApprovalPending = "ApprovalPending"
	// ReasonApprovalDenied indicates the approval has been denied.
	ReasonApprovalDenied = "ApprovalDenied"
	// ReasonApprovalRequestDenied indicates the approval request itself was denied.
	ReasonApprovalRequestDenied = "ApprovalRequestDenied"
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

// ClearApprovalPendingReady updates the Ready condition on the object if it is
// still set to ApprovalPending. This should be called after approval is granted
// to ensure downstream blockers don't leave a stale Ready reason.
// Returns true if the condition was updated.
func ClearApprovalPendingReady(obj types.Object) bool {
	ready := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
	if ready != nil && ready.Reason == ReasonApprovalPending {
		return obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonProcessing, "Approval granted, provisioning in progress"))
	}
	return false
}
