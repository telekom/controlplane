// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApprovalRequest Webhook", func() {

	Context("When creating ApprovalRequest under Defaulting Webhook", func() {
		It("Should fill in the default value if a required field is empty", func() {

			// TODO(user): Add your logic here

		})

		It("Should add an AUTO-approved decision for Auto strategy", func() {
			defaulter := ApprovalRequestCustomDefaulter{}
			ar := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
				},
			}

			err := defaulter.Default(context.Background(), ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(ar.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			Expect(ar.Spec.Decisions).To(HaveLen(1))
			Expect(ar.Spec.Decisions[0].Name).To(Equal("System"))
			Expect(ar.Spec.Decisions[0].Comment).To(Equal(approvalv1.AutoApprovedComment))
			Expect(ar.Spec.Decisions[0].ResultingState).To(Equal(approvalv1.ApprovalStateGranted))
		})

		It("Should not duplicate the AUTO-approved decision on repeated defaulting", func() {
			defaulter := ApprovalRequestCustomDefaulter{}
			ar := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					Decisions: []approvalv1.Decision{
						{Name: "System", Comment: approvalv1.AutoApprovedComment, ResultingState: approvalv1.ApprovalStateGranted},
					},
				},
			}

			err := defaulter.Default(context.Background(), ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(ar.Spec.Decisions).To(HaveLen(1))
		})

		It("Should not add AUTO-approved decision for non-Auto strategies", func() {
			defaulter := ApprovalRequestCustomDefaulter{}
			ar := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
				},
			}

			err := defaulter.Default(context.Background(), ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(ar.Spec.Decisions).To(BeEmpty())
		})
	})

	Context("When creating ApprovalRequest under Validating Webhook", func() {
		It("Should deny if a required field is empty", func() {

			// TODO(user): Add your logic here

		})

		It("Should admit if all required fields are provided", func() {

			// TODO(user): Add your logic here

		})
	})

	Context("decision requirement on state change", func() {
		var validator ApprovalRequestCustomValidator

		// helper to build an ApprovalRequest with the given strategy, spec state, and last state
		makeAR := func(strategy approvalv1.ApprovalStrategy, specState approvalv1.ApprovalState, lastState approvalv1.ApprovalState, decisions []approvalv1.Decision) *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  strategy,
					State:     specState,
					Decisions: decisions,
				},
				Status: approvalv1.ApprovalRequestStatus{
					LastState: lastState,
					AvailableTransitions: approvalv1.AvailableTransitions{
						{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateGranted},
						{Action: approvalv1.ApprovalActionDeny, To: approvalv1.ApprovalStateRejected},
						{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateSemigranted},
					},
				},
			}
		}

		oneDecision := []approvalv1.Decision{
			{Name: "Alice", Email: "alice@telekom.de", Comment: "Approved"},
		}

		It("should reject Simple Pending->Granted with zero decisions", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should reject Simple Pending->Rejected with zero decisions", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateRejected, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should accept Simple Pending->Granted with one decision", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, approvalv1.ApprovalStatePending, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept Simple Pending->Rejected with one decision", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateRejected, approvalv1.ApprovalStatePending, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject FourEyes Pending->Semigranted with zero decisions", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should accept FourEyes Pending->Semigranted with one decision", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStatePending, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not require decisions for Auto strategy state changes", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should also accept Auto strategy state changes with auto-approved decision", func() {
			autoDecision := []approvalv1.Decision{
				{Name: "System", Comment: approvalv1.AutoApprovedComment, ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, approvalv1.ApprovalStatePending, autoDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not require decisions when state has not changed", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("validateDistinctDeciders", func() {

		It("should accept two decisions from distinct deciders", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Looks good"},
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Approved"},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject two decisions from the same decider (exact match)", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First approval"},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Second approval"},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})

		It("should reject two decisions from the same decider (case-insensitive)", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "Alice@Telekom.DE", Comment: "First"},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Second"},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})

		It("should reject when fewer than two decisions are provided", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Only one"},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least two decisions"))
		})

		It("should reject when no decisions are provided", func() {
			decisions := []approvalv1.Decision{}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least two decisions"))
		})

		It("should only check the last two decisions when more than two exist", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First"},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Rejected then re-decided"},
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Final approval"},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject when last two of many decisions are from the same person", func() {
			decisions := []approvalv1.Decision{
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Early decision"},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Penultimate"},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Last"},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})
	})
})
