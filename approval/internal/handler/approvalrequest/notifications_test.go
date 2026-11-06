// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

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

	It("names the ApprovalRequest instead of dumping it when a notification fails", func() {
		approvalReq := &approvalv1.ApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "ar", Namespace: "ns", ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "dump-marker"}}},
			Spec: approvalv1.ApprovalRequestSpec{
				State:     approvalv1.ApprovalStateGranted,
				Strategy:  approvalv1.ApprovalStrategyAuto,
				Requester: approvalv1.Requester{TeamName: "no-group", ApplicationRef: &ctypes.TypedObjectRef{}},
				Decider:   approvalv1.Decider{TeamName: "no-group", ApplicationRef: &ctypes.TypedObjectRef{}},
			},
			Status: approvalv1.ApprovalRequestStatus{LastState: approvalv1.ApprovalStatePending},
		}
		ctx := contextutil.WithEnv(contextutil.WithRecorder(context.Background(), record.NewFakeRecorder(10)), "test")

		err := (&ApprovalRequestHandler{}).CreateOrUpdate(ctx, approvalReq)

		Expect(err).To(MatchError(ContainSubstring(`approval request "ar" in namespace "ns"`)))
		Expect(err.Error()).ToNot(ContainSubstring("dump-marker"))
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
