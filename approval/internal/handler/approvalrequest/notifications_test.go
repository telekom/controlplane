// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"testing"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApprovalRequestHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApprovalRequest Handler Suite")
}

var _ = Describe("handleNotifications", func() {
	refA := ctypes.ObjectRef{Namespace: "ns", Name: "a"}
	refB := ctypes.ObjectRef{Namespace: "ns", Name: "b"}

	It("deduplicates existing notification refs even when the state did not change", func() {
		approvalReq := &approvalv1.ApprovalRequest{
			Spec:   approvalv1.ApprovalRequestSpec{State: approvalv1.ApprovalStatePending},
			Status: approvalv1.ApprovalRequestStatus{LastState: approvalv1.ApprovalStatePending},
		}
		approvalReq.Status.NotificationRefs = []ctypes.ObjectRef{refA, refB, refA, refB}

		Expect(handleNotifications(context.Background(), approvalReq)).To(Succeed())

		Expect(approvalReq.Status.NotificationRefs).To(Equal([]ctypes.ObjectRef{refA, refB}))
		Expect(approvalReq.Spec.State).To(Equal(approvalv1.ApprovalStatePending))
		Expect(approvalReq.Status.LastState).To(Equal(approvalv1.ApprovalStatePending))
	})

	It("leaves an already unique list untouched", func() {
		approvalReq := &approvalv1.ApprovalRequest{
			Spec:   approvalv1.ApprovalRequestSpec{State: approvalv1.ApprovalStatePending},
			Status: approvalv1.ApprovalRequestStatus{LastState: approvalv1.ApprovalStatePending},
		}
		approvalReq.Status.NotificationRefs = []ctypes.ObjectRef{refA, refB}

		Expect(handleNotifications(context.Background(), approvalReq)).To(Succeed())

		Expect(approvalReq.Status.NotificationRefs).To(Equal([]ctypes.ObjectRef{refA, refB}))
	})
})
