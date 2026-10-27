// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/test"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SetPendingConditions", func() {
	var (
		owner   *test.TestResource
		hint    *contextutil.ReconcileHint
		hintCtx context.Context
	)

	newBuilder := func(state approvalv1.ApprovalState, decisions ...approvalv1.Decision) ApprovalBuilder {
		return &approvalBuilder{Request: &approvalv1.ApprovalRequest{
			Spec: approvalv1.ApprovalRequestSpec{State: state, Decisions: decisions},
		}}
	}
	grantedAt := func(t time.Time) approvalv1.Decision {
		return approvalv1.Decision{Name: "System", ResultingState: approvalv1.ApprovalStateGranted, Timestamp: &metav1.Time{Time: t}}
	}
	// A grant time far past the bound.
	pastBound := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ready := func() *metav1.Condition {
		return meta.FindStatusCondition(owner.GetConditions(), condition.ConditionTypeReady)
	}
	processing := func() *metav1.Condition {
		return meta.FindStatusCondition(owner.GetConditions(), condition.ConditionTypeProcessing)
	}

	BeforeEach(func() {
		owner = test.NewObject("owner", testNamespace)
		hint = &contextutil.ReconcileHint{}
		hintCtx = contextutil.WithReconcileHint(context.Background(), hint)
	})

	It("blocks while the ApprovalRequest waits for a decision", func() {
		SetPendingConditions(hintCtx, owner, newBuilder(approvalv1.ApprovalStatePending), "Waiting for approval decision")

		Expect(ready().Status).To(Equal(metav1.ConditionFalse))
		Expect(ready().Reason).To(Equal(ReasonApprovalPending))
		Expect(ready().Message).To(Equal("Waiting for approval decision"))
		Expect(processing().Status).To(Equal(metav1.ConditionFalse))
		Expect(processing().Reason).To(Equal(condition.ReasonBlocked))
		Expect(processing().Message).To(Equal("Waiting for approval decision"))
		Expect(hint.RequeueAfter).To(BeNil())
	})

	It("blocks while a FourEyes ApprovalRequest is only semigranted", func() {
		SetPendingConditions(hintCtx, owner, newBuilder(approvalv1.ApprovalStateSemigranted), "Waiting for approval decision")

		Expect(ready().Reason).To(Equal(ReasonApprovalPending))
		Expect(processing().Reason).To(Equal(condition.ReasonBlocked))
	})

	DescribeTable("waits for the second approval of a semigranted Approval bound to the granted ApprovalRequest",
		func(boundTo, expectedMessage string) {
			ab := newBuilder(approvalv1.ApprovalStateGranted, grantedAt(pastBound))
			ab.GetApprovalRequest().Name = "ar"
			ab.(*approvalBuilder).Approval = &approvalv1.Approval{Spec: approvalv1.ApprovalSpec{
				State:           approvalv1.ApprovalStateSemigranted,
				ApprovedRequest: &ctypes.ObjectRef{Name: boundTo},
			}}
			SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

			Expect(ready().Reason).To(Equal(ReasonApprovalPending))
			Expect(processing().Reason).To(Equal(condition.ReasonBlocked))
			Expect(strings.HasPrefix(ready().Message, expectedMessage)).To(BeTrue(), ready().Message)
		},
		Entry("bound to the request", "ar", "Waiting for approval decision"),
		// The approval controller replaces the Approval of another request.
		Entry("bound to another request", "other", "ApprovalRequest ar was granted at"),
	)

	It("reports processing while a granted ApprovalRequest waits for its Approval", func() {
		ab := newBuilder(approvalv1.ApprovalStateGranted, grantedAt(time.Now().Add(-time.Minute)))
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

		Expect(ready().Status).To(Equal(metav1.ConditionFalse))
		Expect(ready().Reason).To(Equal(condition.ReasonProcessing))
		Expect(ready().Message).To(Equal("Approval granted, waiting for the Approval to be issued"))
		Expect(processing().Status).To(Equal(metav1.ConditionTrue))
		Expect(processing().Reason).To(Equal(condition.ReasonProcessing))
		Expect(processing().Message).To(Equal("Approval granted, waiting for the Approval to be issued"))

		By("requeueing after at most 30s")
		Expect(hint.RequeueAfter).ToNot(BeNil())
		Expect(*hint.RequeueAfter).To(BeNumerically(">=", 30*time.Second))
		Expect(*hint.RequeueAfter).To(BeNumerically("<=", 33*time.Second))
	})

	It("requeues for the remaining time just before the timeout", func() {
		ab := newBuilder(approvalv1.ApprovalStateGranted, grantedAt(time.Now().Add(-approvalIssueTimeout+10*time.Second)))
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

		Expect(ready().Reason).To(Equal(condition.ReasonProcessing))
		Expect(processing().Status).To(Equal(metav1.ConditionTrue))
		Expect(hint.RequeueAfter).ToNot(BeNil())
		Expect(*hint.RequeueAfter).To(BeNumerically(">=", 9*time.Second))
		Expect(*hint.RequeueAfter).To(BeNumerically("<=", 11*time.Second))
	})

	It("blocks once the Approval is not issued within the timeout", func() {
		ab := newBuilder(approvalv1.ApprovalStateGranted, grantedAt(pastBound))
		ab.GetApprovalRequest().Name = "apisubscription--sub--abc"
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

		Expect(ready().Status).To(Equal(metav1.ConditionFalse))
		Expect(ready().Reason).To(Equal(ReasonApprovalPending))
		Expect(ready().Message).To(Equal("ApprovalRequest apisubscription--sub--abc was granted at 2026-01-02T03:04:05Z, but no Approval was issued; check the approval controller"))
		Expect(processing().Status).To(Equal(metav1.ConditionFalse))
		Expect(processing().Reason).To(Equal(condition.ReasonBlocked))
		Expect(processing().Message).To(Equal(ready().Message))
		Expect(hint.RequeueAfter).To(BeNil())
	})

	It("keeps the same conditions on every reconcile past the bound", func() {
		ab := newBuilder(approvalv1.ApprovalStateGranted, grantedAt(time.Now().Add(-approvalIssueTimeout-10*time.Second)))
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")
		first := slices.Clone(owner.GetConditions())

		time.Sleep(1100 * time.Millisecond)
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

		Expect(owner.GetConditions()).To(Equal(first))
	})

	It("appends the current Ready=False message of the ApprovalRequest once blocked", func() {
		ab := newBuilder(approvalv1.ApprovalStateGranted, grantedAt(pastBound))
		request := ab.GetApprovalRequest()
		request.Name = "ar"
		request.Generation = 2
		request.SetCondition(metav1.Condition{
			Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: condition.ReasonError,
			Message: "failed to create or update approval", ObservedGeneration: 2,
		})
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

		Expect(ready().Message).To(Equal("ApprovalRequest ar was granted at 2026-01-02T03:04:05Z, but no Approval was issued; check the approval controller: failed to create or update approval"))
	})

	DescribeTable("does not append a Ready message of the ApprovalRequest that is not a current error",
		func(reason string, observedGeneration int64) {
			ab := newBuilder(approvalv1.ApprovalStateGranted, grantedAt(pastBound))
			request := ab.GetApprovalRequest()
			request.Name = "ar"
			request.Generation = 2
			request.SetCondition(metav1.Condition{
				Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: reason,
				Message: "some message", ObservedGeneration: observedGeneration,
			})
			SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

			Expect(ready().Message).To(Equal("ApprovalRequest ar was granted at 2026-01-02T03:04:05Z, but no Approval was issued; check the approval controller"))
		},
		Entry("stale error", condition.ReasonError, int64(1)),
		Entry("current non-error reason", "Pending", int64(2)),
	)

	It("truncates a long error of the ApprovalRequest", func() {
		ab := newBuilder(approvalv1.ApprovalStateGranted, grantedAt(pastBound))
		request := ab.GetApprovalRequest()
		request.Name = "ar"
		request.SetCondition(metav1.Condition{
			Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: condition.ReasonError,
			Message: strings.Repeat("x", 1000),
		})
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

		prefix := "ApprovalRequest ar was granted at 2026-01-02T03:04:05Z, but no Approval was issued; check the approval controller: "
		Expect(ready().Message).To(Equal(prefix + strings.Repeat("x", 255) + "…"))
	})

	It("measures the timeout from the latest grant", func() {
		ab := newBuilder(approvalv1.ApprovalStateGranted,
			grantedAt(time.Now().Add(-time.Hour)),
			approvalv1.Decision{Name: "Bob", ResultingState: approvalv1.ApprovalStateRejected, Timestamp: &metav1.Time{Time: time.Now()}},
			grantedAt(time.Now().Add(-time.Minute)),
		)
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

		Expect(ready().Reason).To(Equal(condition.ReasonProcessing))
	})

	It("falls back to the creation time of the ApprovalRequest", func() {
		ab := newBuilder(approvalv1.ApprovalStateGranted, approvalv1.Decision{Name: "System", ResultingState: approvalv1.ApprovalStateGranted})
		ab.GetApprovalRequest().CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * approvalIssueTimeout))
		SetPendingConditions(hintCtx, owner, ab, "Waiting for approval decision")

		Expect(processing().Reason).To(Equal(condition.ReasonBlocked))
	})

	It("starts the timeout now when the grant time is unknown", func() {
		SetPendingConditions(hintCtx, owner, newBuilder(approvalv1.ApprovalStateGranted), "Waiting for approval decision")

		Expect(ready().Reason).To(Equal(condition.ReasonProcessing))
		Expect(processing().Status).To(Equal(metav1.ConditionTrue))
	})

	DescribeTable("ClearApprovalPendingReady replaces the Ready it set once the Approval is granted",
		func(state approvalv1.ApprovalState, grant time.Time) {
			SetPendingConditions(hintCtx, owner, newBuilder(state, grantedAt(grant)), "Waiting for approval decision")

			Expect(ClearApprovalPendingReady(owner)).To(BeTrue())
			Expect(ready().Reason).To(Equal(condition.ReasonProcessing))
			Expect(ready().Message).To(Equal("Approval granted, provisioning in progress"))
		},
		Entry("waiting for a decision", approvalv1.ApprovalStatePending, time.Now()),
		Entry("waiting for the Approval", approvalv1.ApprovalStateGranted, time.Now()),
		Entry("Approval not issued", approvalv1.ApprovalStateGranted, time.Now().Add(-2*approvalIssueTimeout)),
	)
})
