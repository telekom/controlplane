// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("liftsScopedRevocation", func() {
	revokedAt := metav1.NewTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	before := metav1.NewTime(revokedAt.Add(-time.Hour))
	after := metav1.NewTime(revokedAt.Add(time.Hour))

	// revoked returns an Approval in state, as the approval controller leaves it
	// (Status.LastState follows Spec.State), granted before and then revoked
	// with a decision resulting in revokingState at at, followed by later.
	revoked := func(state, revokingState approvalv1.ApprovalState, at *metav1.Time, later ...approvalv1.Decision) *approvalv1.Approval {
		a := &approvalv1.Approval{}
		a.Spec.State = state
		a.Status.LastState = state
		a.Spec.ApprovedRequest = &ctypes.ObjectRef{Name: "old", Namespace: "default", UID: "old-uid"}
		a.Spec.Decisions = append([]approvalv1.Decision{
			{Name: "Alice", ResultingState: approvalv1.ApprovalStateGranted, Timestamp: &before},
			{Name: "Alice", ResultingState: revokingState, Timestamp: at},
		}, later...)
		return a
	}
	expiry := approvalv1.Decision{Name: approvalv1.SystemDecisionName, ResultingState: approvalv1.ApprovalStateExpired, Timestamp: &after}
	request := func(uid ktypes.UID, strategy approvalv1.ApprovalStrategy, state approvalv1.ApprovalState, decisions ...approvalv1.Decision) *approvalv1.ApprovalRequest {
		ar := &approvalv1.ApprovalRequest{}
		ar.UID = uid
		ar.Spec.Strategy = strategy
		ar.Spec.State = state
		ar.Spec.Decisions = decisions
		return ar
	}
	grantAt := func(at *metav1.Time) approvalv1.Decision {
		return approvalv1.Decision{Name: "Bob", ResultingState: approvalv1.ApprovalStateGranted, Timestamp: at}
	}
	rejected := revoked(approvalv1.ApprovalStateRejected, approvalv1.ApprovalStateRejected, &revokedAt)
	// expiredAfterSilentSuspension is an Approval suspended without a decision,
	// then expired: the expiration handler recorded the suspension at the expiry.
	expiredAfterSilentSuspension := &approvalv1.Approval{}
	expiredAfterSilentSuspension.Spec.State = approvalv1.ApprovalStateExpired
	expiredAfterSilentSuspension.Status.LastState = approvalv1.ApprovalStateExpired
	expiredAfterSilentSuspension.Spec.ApprovedRequest = &ctypes.ObjectRef{Name: "old", Namespace: "default", UID: "old-uid"}
	expiredAfterSilentSuspension.Spec.Decisions = []approvalv1.Decision{
		{Name: "Alice", ResultingState: approvalv1.ApprovalStateGranted, Timestamp: &before},
		{Name: approvalv1.SystemDecisionName, ResultingState: approvalv1.ApprovalStateSuspended, Timestamp: &revokedAt},
		{Name: approvalv1.SystemDecisionName, ResultingState: approvalv1.ApprovalStateExpired, Timestamp: &revokedAt},
	}

	// halfRegranted is a revoked FourEyes Approval with the first approval of a
	// re-grant.
	halfRegranted := revoked(approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStateRejected, &before,
		approvalv1.Decision{Name: "Carol", ResultingState: approvalv1.ApprovalStateSemigranted, Timestamp: &revokedAt})

	DescribeTable("decides whether a request replaces a revocation",
		func(approval *approvalv1.Approval, ar *approvalv1.ApprovalRequest, lifts bool) {
			Expect(isScopedApprovalRevoked(approval)).To(BeTrue())
			Expect(liftsScopedRevocation(approval, ar)).To(Equal(lifts))
		},
		Entry("Rejected, newer request granted by a person after the revocation", rejected,
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), true),
		Entry("Suspended, newer request granted by a person after the suspension",
			revoked(approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateSuspended, &revokedAt),
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), true),
		Entry("Expired after a suspension, newer request granted by a person after the suspension",
			revoked(approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateSuspended, &revokedAt, expiry),
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), true),
		Entry("Expired after a suspension without a decision, newer request granted by a person after the expiry",
			expiredAfterSilentSuspension,
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), true),
		Entry("Expired after a suspension without a decision, grant made before the expiry",
			expiredAfterSilentSuspension,
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&before)), false),
		Entry("FourEyes request fully granted after the revocation", rejected,
			request("new-uid", approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Bob", ResultingState: approvalv1.ApprovalStateSemigranted, Timestamp: &after},
				grantAt(&after)), true),
		Entry("FourEyes request whose first approval predates the revocation", rejected,
			request("new-uid", approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Bob", ResultingState: approvalv1.ApprovalStateSemigranted, Timestamp: &before},
				grantAt(&after)), false),
		Entry("FourEyes request whose second approval is dated before the revocation", rejected,
			request("new-uid", approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateGranted,
				approvalv1.Decision{Name: "Bob", ResultingState: approvalv1.ApprovalStateSemigranted, Timestamp: &after},
				grantAt(&before)), false),
		Entry("FourEyes request granted without a first approval", rejected,
			request("new-uid", approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateGranted, grantAt(&after)), false),
		Entry("Semigranted after a revocation, newer request granted by a person after the first re-grant approval",
			halfRegranted,
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), true),
		Entry("Semigranted after a revocation, automatic grant", halfRegranted,
			request("new-uid", approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, grantAt(&after)), false),
		Entry("FourEyes request only Semigranted", rejected,
			request("new-uid", approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted,
				approvalv1.Decision{Name: "Bob", ResultingState: approvalv1.ApprovalStateSemigranted, Timestamp: &after}), false),
		Entry("automatic grant", rejected,
			request("new-uid", approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, grantAt(&after)), false),
		Entry("grant made before the revocation", rejected,
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&before)), false),
		Entry("grant made in the same second as the revocation", rejected,
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&revokedAt)), false),
		Entry("grant without a timestamp", rejected,
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(nil)), false),
		Entry("revocation without a timestamp",
			revoked(approvalv1.ApprovalStateRejected, approvalv1.ApprovalStateRejected, nil),
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), false),
		Entry("Suspended without a decision after an earlier, lifted revocation",
			revoked(approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateRejected, &before,
				approvalv1.Decision{Name: "Carol", ResultingState: approvalv1.ApprovalStateGranted, Timestamp: &revokedAt}),
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), false),
		Entry("Rejected, but the last decision is a different revocation",
			revoked(approvalv1.ApprovalStateRejected, approvalv1.ApprovalStateSuspended, &revokedAt),
			request("new-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), false),
		Entry("the request the Approval was granted for", rejected,
			request("old-uid", approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, grantAt(&after)), false),
	)
})
