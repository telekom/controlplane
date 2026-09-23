// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"testing"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApprovalHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Approval Handler Suite")
}

var _ = Describe("handleNotifications", func() {
	refA := ctypes.ObjectRef{Namespace: "ns", Name: "a"}
	refB := ctypes.ObjectRef{Namespace: "ns", Name: "b"}

	It("deduplicates existing notification refs even when the state did not change", func() {
		approval := &approvalv1.Approval{
			Spec:   approvalv1.ApprovalSpec{State: approvalv1.ApprovalStateGranted},
			Status: approvalv1.ApprovalStatus{LastState: approvalv1.ApprovalStateGranted},
		}
		approval.Status.NotificationRefs = []ctypes.ObjectRef{refA, refB, refA}

		Expect(handleNotifications(context.Background(), approval)).To(Succeed())

		Expect(approval.Status.NotificationRefs).To(Equal([]ctypes.ObjectRef{refA, refB}))
		Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		Expect(approval.Status.LastState).To(Equal(approvalv1.ApprovalStateGranted))
	})

	It("leaves an already unique list untouched", func() {
		approval := &approvalv1.Approval{
			Spec:   approvalv1.ApprovalSpec{State: approvalv1.ApprovalStateGranted},
			Status: approvalv1.ApprovalStatus{LastState: approvalv1.ApprovalStateGranted},
		}
		approval.Status.NotificationRefs = []ctypes.ObjectRef{refA, refB}

		Expect(handleNotifications(context.Background(), approval)).To(Succeed())

		Expect(approval.Status.NotificationRefs).To(Equal([]ctypes.ObjectRef{refA, refB}))
	})
})
