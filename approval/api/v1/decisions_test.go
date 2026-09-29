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
