// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		makeAR := func(strategy approvalv1.ApprovalStrategy, specState approvalv1.ApprovalState, decisions []approvalv1.Decision) *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  strategy,
					State:     specState,
					Decisions: decisions,
				},
				Status: approvalv1.ApprovalRequestStatus{
					LastState: approvalv1.ApprovalStatePending,
				},
			}
		}

		oneDecision := []approvalv1.Decision{
			{Name: "Alice", Email: "alice@telekom.de", Comment: "Approved", ResultingState: approvalv1.ApprovalStateGranted},
		}

		It("should reject Simple Pending->Granted with zero decisions", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should reject Simple Pending->Rejected with zero decisions", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateRejected, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should accept Simple Pending->Granted with one decision", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept Simple Pending->Rejected with one decision", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateRejected, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject FourEyes Pending->Semigranted with zero decisions", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should accept FourEyes Pending->Semigranted with one decision", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not require decisions for Auto strategy state changes", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should also accept Auto strategy state changes with auto-approved decision", func() {
			autoDecision := []approvalv1.Decision{
				{Name: "System", Comment: approvalv1.AutoApprovedComment, ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, autoDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not require decisions when state has not changed", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("validateDistinctDeciders", func() {
		It("should accept two decisions from distinct deciders", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Looks good", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Approved", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject two decisions from the same decider (exact match)", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First approval", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Second approval", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})

		It("should reject two decisions from the same decider (case-insensitive)", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "Alice@Telekom.DE", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})

		It("should reject when fewer than two decisions are provided", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Only one", ResultingState: approvalv1.ApprovalStateSemigranted},
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
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Rejected then re-decided", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Final approval", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject when last two of many decisions are from the same person", func() {
			decisions := []approvalv1.Decision{
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Early decision", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Penultimate", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Last", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})

		It("should reject when last decision has empty email", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty email"))
		})

		It("should reject when second-to-last decision has empty email", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty email"))
		})

		It("should reject when both decisions have empty emails", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty email"))
		})

		It("should reject when email is only whitespace", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "   ", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			err := validateDistinctDeciders(decisions)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty email"))
		})
	})

	Context("stateChanged uses oldObj.Spec.State not Status.LastState", func() {
		var validator ApprovalRequestCustomValidator

		It("should not flag as state change when only Status.LastState differs", func() {
			// Both oldObj and newObj have Spec.State=Granted (no real change),
			// but newObj.Status.LastState=Pending (stale). Should NOT trigger decision check.
			oldObj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
				},
			}
			newObj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
				},
				Status: approvalv1.ApprovalRequestStatus{
					LastState: approvalv1.ApprovalStatePending, // stale
				},
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("defaultDecisionFields via ApprovalRequest defaulter", func() {
		It("should populate Timestamp and ResultingState on new decisions", func() {
			defaulter := ApprovalRequestCustomDefaulter{}
			before := time.Now()
			ar := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{
						{Name: "Alice", Email: "alice@telekom.de", Comment: "ok"},
					},
				},
			}

			err := defaulter.Default(context.Background(), ar)
			Expect(err).NotTo(HaveOccurred())

			d := ar.Spec.Decisions[0]
			Expect(d.ResultingState).To(Equal(approvalv1.ApprovalStateGranted))
			Expect(d.Timestamp).NotTo(BeNil())
			Expect(d.Timestamp.Time).To(BeTemporally(">=", before))
			Expect(d.Timestamp.Time).To(BeTemporally("<=", time.Now()))
		})

		It("should not overwrite existing Timestamp or ResultingState", func() {
			defaulter := ApprovalRequestCustomDefaulter{}
			fixedTime := metav1.NewTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			ar := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{
						{
							Name:           "Alice",
							Email:          "alice@telekom.de",
							Comment:        "ok",
							Timestamp:      &fixedTime,
							ResultingState: approvalv1.ApprovalStateSemigranted,
						},
					},
				},
			}

			err := defaulter.Default(context.Background(), ar)
			Expect(err).NotTo(HaveOccurred())

			d := ar.Spec.Decisions[0]
			Expect(d.ResultingState).To(Equal(approvalv1.ApprovalStateSemigranted))
			Expect(d.Timestamp.Time).To(Equal(fixedTime.Time))
		})

		It("should populate fields for multiple decisions independently", func() {
			defaulter := ApprovalRequestCustomDefaulter{}
			fixedTime := metav1.NewTime(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
			ar := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyFourEyes,
					State:    approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{
						{
							Name:           "Alice",
							Email:          "alice@telekom.de",
							Comment:        "First",
							Timestamp:      &fixedTime,
							ResultingState: approvalv1.ApprovalStateSemigranted,
						},
						{
							Name:    "Bob",
							Email:   "bob@telekom.de",
							Comment: "Second",
						},
					},
				},
			}

			err := defaulter.Default(context.Background(), ar)
			Expect(err).NotTo(HaveOccurred())

			Expect(ar.Spec.Decisions[0].Timestamp.Time).To(Equal(fixedTime.Time))
			Expect(ar.Spec.Decisions[0].ResultingState).To(Equal(approvalv1.ApprovalStateSemigranted))

			Expect(ar.Spec.Decisions[1].Timestamp).NotTo(BeNil())
			Expect(ar.Spec.Decisions[1].ResultingState).To(Equal(approvalv1.ApprovalStateGranted))
		})

		It("should not mutate Auto AR that is already in terminal state", func() {
			defaulter := ApprovalRequestCustomDefaulter{}
			ar := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  approvalv1.ApprovalStrategyAuto,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{},
				},
			}

			err := defaulter.Default(context.Background(), ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(ar.Spec.Decisions).To(BeEmpty())
			Expect(ar.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
		})
	})

	Context("terminal state spec-freeze", func() {
		var validator ApprovalRequestCustomValidator

		grantedAR := func() *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
					Action:   "subscribe",
					Requester: approvalv1.Requester{
						TeamName: "team-a",
					},
					Decider: approvalv1.Decider{
						TeamName: "team-b",
					},
					Decisions: []approvalv1.Decision{
						{Name: "Alice", Email: "alice@telekom.de", Comment: "Approved", ResultingState: approvalv1.ApprovalStateGranted},
					},
				},
			}
		}

		rejectedAR := func(strategy approvalv1.ApprovalStrategy) *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: strategy,
					State:    approvalv1.ApprovalStateRejected,
					Action:   "subscribe",
					Requester: approvalv1.Requester{
						TeamName: "team-a",
					},
					Decider: approvalv1.Decider{
						TeamName: "team-b",
					},
					Decisions: []approvalv1.Decision{
						{Name: "Bob", Email: "bob@telekom.de", Comment: "Denied", ResultingState: approvalv1.ApprovalStateRejected},
					},
				},
			}
		}

		// --- Granted AR is frozen ---

		It("should reject state change on a Granted AR", func() {
			oldObj := grantedAR()
			newObj := grantedAR()
			newObj.Spec.State = approvalv1.ApprovalStatePending
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("terminal state"))
		})

		It("should reject adding decisions to a Granted AR", func() {
			oldObj := grantedAR()
			newObj := grantedAR()
			newObj.Spec.Decisions = append(newObj.Spec.Decisions, approvalv1.Decision{
				Name: "Eve", Email: "eve@telekom.de", Comment: "Extra decision",
			})
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("terminal state"))
		})

		It("should reject changing decisions on a Granted Auto AR", func() {
			oldObj := grantedAR()
			oldObj.Spec.Strategy = approvalv1.ApprovalStrategyAuto

			newObj := grantedAR()
			newObj.Spec.Strategy = approvalv1.ApprovalStrategyAuto
			newObj.Spec.Decisions = []approvalv1.Decision{
				{Name: "Hacker", Comment: "tampering"},
			}

			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("terminal state"))
		})

		It("should reject changing strategy on a Granted AR", func() {
			oldObj := grantedAR()
			newObj := grantedAR()
			newObj.Spec.Strategy = approvalv1.ApprovalStrategyFourEyes
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("terminal state"))
		})

		It("should allow changing requester on a Granted AR", func() {
			oldObj := grantedAR()
			newObj := grantedAR()
			newObj.Spec.Requester.TeamName = "team-c"
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow no-op update on a Granted AR (identical spec)", func() {
			oldObj := grantedAR()
			newObj := grantedAR()
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		// --- Rejected AR is NOT frozen (can be re-approved) ---

		It("should allow re-approval of a Rejected Simple AR (Rejected -> Granted)", func() {
			oldObj := rejectedAR(approvalv1.ApprovalStrategySimple)
			newObj := rejectedAR(approvalv1.ApprovalStrategySimple)
			newObj.Spec.State = approvalv1.ApprovalStateGranted
			newObj.Spec.Decisions = append(newObj.Spec.Decisions, approvalv1.Decision{
				Name: "Alice", Email: "alice@telekom.de", Comment: "Re-approved", ResultingState: approvalv1.ApprovalStateGranted,
			})
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow re-approval of a Rejected FourEyes AR (Rejected -> Semigranted)", func() {
			oldObj := rejectedAR(approvalv1.ApprovalStrategyFourEyes)
			newObj := rejectedAR(approvalv1.ApprovalStrategyFourEyes)
			newObj.Spec.State = approvalv1.ApprovalStateSemigranted
			newObj.Spec.Decisions = append(newObj.Spec.Decisions, approvalv1.Decision{
				Name: "Alice", Email: "alice@telekom.de", Comment: "First re-approval", ResultingState: approvalv1.ApprovalStateSemigranted,
			})
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow modifying decisions on a Rejected AR", func() {
			oldObj := rejectedAR(approvalv1.ApprovalStrategySimple)
			newObj := rejectedAR(approvalv1.ApprovalStrategySimple)
			newObj.Spec.Decisions = append(newObj.Spec.Decisions, approvalv1.Decision{
				Name: "Eve", Email: "eve@telekom.de", Comment: "Additional note",
			})
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow no-op update on a Rejected AR (identical spec)", func() {
			oldObj := rejectedAR(approvalv1.ApprovalStrategySimple)
			newObj := rejectedAR(approvalv1.ApprovalStrategySimple)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("on-the-fly FSM validation (Bug 1+2 fix)", func() {
		var validator ApprovalRequestCustomValidator

		makeAR := func(strategy approvalv1.ApprovalStrategy, specState approvalv1.ApprovalState, decisions []approvalv1.Decision) *approvalv1.ApprovalRequest {
			return &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  strategy,
					State:     specState,
					Decisions: decisions,
				},
			}
		}

		oneDecision := []approvalv1.Decision{
			{Name: "Alice", Email: "alice@telekom.de", Comment: "ok", ResultingState: approvalv1.ApprovalStateGranted},
		}

		It("should reject invalid transition even when Status.AvailableTransitions is nil (Bug 2)", func() {
			// Before the fix, nil AvailableTransitions skipped the FSM check entirely.
			// Simple strategy: Pending -> Semigranted is NOT a valid transition.
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, oneDecision)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateSemigranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid state transition"))
		})

		It("should reject invalid transition regardless of stale Status.AvailableTransitions (Bug 1)", func() {
			// FourEyes: Rejected -> Granted is NOT valid (must go Rejected -> Semigranted first).
			// Even if stale AvailableTransitions listed Granted, the on-the-fly FSM should block it.
			oldObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateRejected, oneDecision)
			oldObj.Status.AvailableTransitions = approvalv1.AvailableTransitions{
				{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateGranted}, // stale!
			}
			newObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateGranted, oneDecision)
			newObj.Status.AvailableTransitions = approvalv1.AvailableTransitions{
				{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateGranted}, // stale!
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid state transition"))
		})

		It("should allow valid Simple transitions with nil Status", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, oneDecision)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow valid FourEyes Pending -> Semigranted with nil Status", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStatePending, oneDecision)
			newObj := makeAR(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject Rejected -> Pending for Simple strategy (no such transition)", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateRejected, oneDecision)
			newObj := makeAR(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid state transition"))
		})

		It("should skip FSM check for Auto strategy (empty FSM)", func() {
			oldObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStatePending, nil)
			newObj := makeAR(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("broadened distinct-decider check (Bug 3 fix)", func() {
		var validator ApprovalRequestCustomValidator

		It("should enforce distinct deciders on Semigranted -> Granted (original case)", func() {
			twoSameDeciders := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyFourEyes,
					State:    approvalv1.ApprovalStateSemigranted,
				},
			}
			newObj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  approvalv1.ApprovalStrategyFourEyes,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: twoSameDeciders,
				},
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})

		It("should not enforce distinct deciders for non-Granted FourEyes transitions", func() {
			// Pending -> Semigranted should NOT trigger the distinct-decider check
			oneDecision := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Approved", ResultingState: approvalv1.ApprovalStateSemigranted},
			}
			oldObj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategyFourEyes,
					State:    approvalv1.ApprovalStatePending,
				},
			}
			newObj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  approvalv1.ApprovalStrategyFourEyes,
					State:     approvalv1.ApprovalStateSemigranted,
					Decisions: oneDecision,
				},
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not enforce distinct deciders for Simple strategy Granted transitions", func() {
			oneDecision := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Approved", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStatePending,
				},
			}
			newObj := &approvalv1.ApprovalRequest{
				Spec: approvalv1.ApprovalRequestSpec{
					Strategy:  approvalv1.ApprovalStrategySimple,
					State:     approvalv1.ApprovalStateGranted,
					Decisions: oneDecision,
				},
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
