// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// buildDecisions creates n decisions named "d0".."d(n-1)" with no Timestamp and
// no ResultingState, so defaulting is observable.
func buildDecisions(n int) []approvalv1.Decision {
	decisions := make([]approvalv1.Decision, 0, n)
	for i := 0; i < n; i++ {
		decisions = append(decisions, approvalv1.Decision{Name: fmt.Sprintf("d%d", i)})
	}
	return decisions
}

func namesOf(decisions []approvalv1.Decision) []string {
	names := make([]string, 0, len(decisions))
	for _, d := range decisions {
		names = append(names, d.Name)
	}
	return names
}

var _ = Describe("Decision trimming in the mutating webhooks", func() {
	Context("ApprovalCustomDefaulter", func() {
		It("Should keep only the newest MaxDecisions and default their fields", func() {
			defaulter := ApprovalCustomDefaulter{}
			a := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy:  approvalv1.ApprovalStrategySimple,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: buildDecisions(7),
				},
			}

			Expect(defaulter.Default(context.Background(), a)).To(Succeed())

			Expect(a.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))
			Expect(namesOf(a.Spec.Decisions)).To(Equal([]string{"d2", "d3", "d4", "d5", "d6"}))
			for _, d := range a.Spec.Decisions {
				Expect(d.Timestamp).NotTo(BeNil(), "decision %q should have a defaulted timestamp", d.Name)
				Expect(d.ResultingState).To(Equal(approvalv1.ApprovalStateGranted))
			}
		})

		It("Should default the newly appended 6th decision before trimming", func() {
			defaulter := ApprovalCustomDefaulter{}
			a := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy:  approvalv1.ApprovalStrategySimple,
					State:     approvalv1.ApprovalStateSuspended,
					Decisions: buildDecisions(6),
				},
			}

			Expect(defaulter.Default(context.Background(), a)).To(Succeed())

			last := a.Spec.Decisions[len(a.Spec.Decisions)-1]
			Expect(last.Name).To(Equal("d5"))
			Expect(last.Timestamp).NotTo(BeNil())
			Expect(last.ResultingState).To(Equal(approvalv1.ApprovalStateSuspended))
		})
	})

	Context("ApprovalRequestCustomDefaulter", func() {
		It("Should keep only the newest MaxDecisions and default their fields", func() {
			defaulter := ApprovalRequestCustomDefaulter{}
			ar := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  approvalv1.ApprovalStrategySimple,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: buildDecisions(7),
				},
			}

			Expect(defaulter.Default(context.Background(), ar)).To(Succeed())

			Expect(ar.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))
			Expect(namesOf(ar.Spec.Decisions)).To(Equal([]string{"d2", "d3", "d4", "d5", "d6"}))
			for _, d := range ar.Spec.Decisions {
				Expect(d.Timestamp).NotTo(BeNil(), "decision %q should have a defaulted timestamp", d.Name)
				Expect(d.ResultingState).To(Equal(approvalv1.ApprovalStateGranted))
			}
		})
	})

	Context("ApprovalRequestCustomValidator on a Granted ApprovalRequest", func() {
		var validator ApprovalRequestCustomValidator

		makeAR := func(decisions []approvalv1.Decision) *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  approvalv1.ApprovalStrategySimple,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: decisions,
				},
			}
		}

		BeforeEach(func() {
			validator = ApprovalRequestCustomValidator{}
		})

		It("Should accept an update that only drops decisions beyond MaxDecisions", func() {
			old := makeAR(buildDecisions(6))
			updated := makeAR(approvalv1.TrimDecisions(buildDecisions(6)))

			warnings, err := validator.ValidateUpdate(context.Background(), old, updated)

			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Should reject an update that alters the most recent decision", func() {
			old := makeAR(buildDecisions(5))
			changed := buildDecisions(5)
			changed[len(changed)-1].Name = "someone-else"
			updated := makeAR(changed)

			_, err := validator.ValidateUpdate(context.Background(), old, updated)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("terminal state"))
		})
	})

	Context("FourEyes validation with a trimmed decision list", func() {
		It("Should accept Semigranted to Granted when the last two deciders differ", func() {
			defaulter := ApprovalCustomDefaulter{}
			validator := ApprovalCustomValidator{OperatorServiceAccount: "system:serviceaccount:system:controller-manager"}

			decisions := buildDecisions(6)
			decisions[4].Email = "first@example.com"
			decisions[5].Email = "second@example.com"

			old := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyFourEyes,
					State:    approvalv1.ApprovalStateSemigranted,
				},
			}
			updated := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy:  approvalv1.ApprovalStrategyFourEyes,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: decisions,
				},
			}

			Expect(defaulter.Default(context.Background(), updated)).To(Succeed())
			Expect(updated.Spec.Decisions).To(HaveLen(approvalv1.MaxDecisions))

			warnings, err := validator.ValidateUpdate(context.Background(), old, updated)

			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})
})
