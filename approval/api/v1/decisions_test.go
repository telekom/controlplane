// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
)

// makeDecisions builds n decisions with names "d0".."d(n-1)" so order is assertable.
func makeDecisions(n int) []approvalv1.Decision {
	decisions := make([]approvalv1.Decision, 0, n)
	for i := 0; i < n; i++ {
		decisions = append(decisions, approvalv1.Decision{
			Name:           fmt.Sprintf("d%d", i),
			ResultingState: approvalv1.ApprovalStateGranted,
		})
	}
	return decisions
}

func decisionNames(decisions []approvalv1.Decision) []string {
	names := make([]string, 0, len(decisions))
	for _, d := range decisions {
		names = append(names, d.Name)
	}
	return names
}

var _ = Describe("TrimDecisions", func() {
	It("returns nil for nil input", func() {
		Expect(approvalv1.TrimDecisions(nil)).To(BeNil())
	})

	It("returns an empty list for empty input", func() {
		Expect(approvalv1.TrimDecisions([]approvalv1.Decision{})).To(BeEmpty())
	})

	It("keeps lists of at most MaxDecisions unchanged", func() {
		for n := 0; n <= approvalv1.MaxDecisions; n++ {
			in := makeDecisions(n)
			Expect(approvalv1.TrimDecisions(in)).To(Equal(in), "n=%d", n)
		}
	})

	It("keeps the last MaxDecisions of a 6-item list", func() {
		got := approvalv1.TrimDecisions(makeDecisions(6))
		Expect(got).To(HaveLen(approvalv1.MaxDecisions))
		Expect(decisionNames(got)).To(Equal([]string{"d1", "d2", "d3", "d4", "d5"}))
	})

	It("keeps the last MaxDecisions of a 12-item list preserving order", func() {
		got := approvalv1.TrimDecisions(makeDecisions(12))
		Expect(got).To(HaveLen(approvalv1.MaxDecisions))
		Expect(decisionNames(got)).To(Equal([]string{"d7", "d8", "d9", "d10", "d11"}))
	})
})

var _ = Describe("AppendDecision", func() {
	newDecision := approvalv1.Decision{Name: "newest", ResultingState: approvalv1.ApprovalStateGranted}

	It("drops the oldest decision on an Approval that is already full", func() {
		approval := &approvalv1.Approval{
			Spec: approvalv1.ApprovalSpec{Decisions: makeDecisions(approvalv1.MaxDecisions)},
		}

		approval.AppendDecision(newDecision)

		Expect(approval.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))
		Expect(decisionNames(approval.Spec.Decisions)).To(Equal([]string{"d1", "d2", "d3", "d4", "newest"}))
	})

	It("drops the oldest decision on an ApprovalRequest that is already full", func() {
		approvalReq := &approvalv1.ApprovalRequest{
			Spec: approvalv1.ApprovalRequestSpec{Decisions: makeDecisions(approvalv1.MaxDecisions)},
		}

		approvalReq.AppendDecision(newDecision)

		Expect(approvalReq.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))
		Expect(decisionNames(approvalReq.Spec.Decisions)).To(Equal([]string{"d1", "d2", "d3", "d4", "newest"}))
	})

	It("simply appends while below the limit", func() {
		approval := &approvalv1.Approval{}

		approval.AppendDecision(newDecision)

		Expect(decisionNames(approval.Spec.Decisions)).To(Equal([]string{"newest"}))
	})
})

var _ = Describe("IsExpiredAfterSuspension", func() {
	decision := func(state approvalv1.ApprovalState) approvalv1.Decision {
		return approvalv1.Decision{Name: "d", ResultingState: state}
	}
	DescribeTable("derives a suspension before the expiry",
		func(state, lastState approvalv1.ApprovalState, decisions []approvalv1.Decision, expected bool) {
			a := &approvalv1.Approval{}
			a.Spec.State = state
			a.Status.LastState = lastState
			a.Spec.Decisions = decisions
			Expect(a.IsExpiredAfterSuspension()).To(Equal(expected))
		},
		Entry("expiry not yet reconciled after a suspension", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateSuspended,
			nil, true),
		Entry("reconciled expiry after a suspension decision", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			[]approvalv1.Decision{decision(approvalv1.ApprovalStateGranted), decision(approvalv1.ApprovalStateSuspended), decision(approvalv1.ApprovalStateExpired)}, true),
		Entry("reconciled expiry after a grant", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			[]approvalv1.Decision{decision(approvalv1.ApprovalStateGranted), decision(approvalv1.ApprovalStateExpired)}, false),
		Entry("reconciled expiry without decisions", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			nil, false),
		Entry("not expired", approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateSuspended,
			[]approvalv1.Decision{decision(approvalv1.ApprovalStateSuspended)}, false),
	)
})

var _ = Describe("IsExpiredAfterGrant", func() {
	decisions := func(states ...approvalv1.ApprovalState) []approvalv1.Decision {
		ds := make([]approvalv1.Decision, 0, len(states))
		for _, state := range states {
			ds = append(ds, approvalv1.Decision{Name: "d", ResultingState: state})
		}
		return ds
	}
	DescribeTable("derives a grant before the expiry",
		func(state, lastState approvalv1.ApprovalState, ds []approvalv1.Decision, expected bool) {
			a := &approvalv1.Approval{}
			a.Spec.State = state
			a.Status.LastState = lastState
			a.Spec.Decisions = ds
			Expect(a.IsExpiredAfterGrant()).To(Equal(expected))
		},
		Entry("expiry not yet reconciled after a grant", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateGranted,
			nil, true),
		Entry("reconciled expiry after a grant", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			decisions(approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateExpired), true),
		Entry("reconciled expiry after a suspension and a resume", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			decisions(approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateExpired), true),
		Entry("reconciled expiry after a suspension", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			decisions(approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateExpired), false),
		Entry("reconciled expiry after a rejection", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			decisions(approvalv1.ApprovalStateRejected, approvalv1.ApprovalStateExpired), false),
		Entry("reconciled expiry without decisions", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			nil, false),
		Entry("reconciled expiry with only the expiry decision", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			decisions(approvalv1.ApprovalStateExpired), false),
		Entry("decision without a resulting state", approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateExpired,
			decisions(approvalv1.ApprovalStateGranted, "", approvalv1.ApprovalStateExpired), false),
		Entry("not expired", approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateGranted,
			decisions(approvalv1.ApprovalStateGranted), false),
	)
})
