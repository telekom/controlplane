// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Approval Webhook", func() {
	Context("When creating Approval under Defaulting Webhook", func() {
		It("Should not modify non-Auto strategy approvals", func() {
			defaulter := ApprovalCustomDefaulter{}
			a := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStatePending,
				},
			}

			err := defaulter.Default(context.Background(), a)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.Spec.State).To(Equal(approvalv1.ApprovalStatePending))
		})
	})

	Context("When creating Approval under Validating Webhook", func() {
		It("Should reject Auto strategy approval not in Granted state on create", func() {
			validator := ApprovalCustomValidator{OperatorServiceAccount: "system:serviceaccount:system:controller-manager"}
			a := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStatePending,
				},
			}

			_, err := validator.ValidateCreate(context.Background(), a)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Auto strategy Approval must be in Granted state"))
		})

		It("Should not reject Auto strategy approval already in Granted state on create", func() {
			validator := ApprovalCustomValidator{OperatorServiceAccount: "system:serviceaccount:system:controller-manager"}
			a := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyAuto,
					State:    approvalv1.ApprovalStateGranted,
				},
			}

			warnings, err := validator.ValidateCreate(context.Background(), a)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("decision requirement on state change", func() {
		var validator ApprovalCustomValidator
		BeforeEach(func() {
			validator = ApprovalCustomValidator{OperatorServiceAccount: "system:serviceaccount:system:controller-manager"}
		})

		// helper to build an Approval with the given strategy, spec state, and last state
		makeApproval := func(strategy approvalv1.ApprovalStrategy, specState, lastState approvalv1.ApprovalState, decisions []approvalv1.Decision) *approvalv1.Approval {
			return &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy:  strategy,
					State:     specState,
					Decisions: decisions,
				},
				Status: approvalv1.ApprovalStatus{
					LastState: lastState,
				},
			}
		}

		oneDecision := []approvalv1.Decision{
			{Name: "Alice", Email: "alice@telekom.de", Comment: "Approved", ResultingState: approvalv1.ApprovalStateGranted},
		}

		It("should reject Simple Pending->Granted with zero decisions", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should reject Simple Pending->Rejected with zero decisions", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateRejected, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should accept Simple Pending->Granted with one decision", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, approvalv1.ApprovalStatePending, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept Simple Pending->Rejected with one decision", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateRejected, approvalv1.ApprovalStatePending, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject FourEyes Pending->Semigranted with zero decisions", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one decision"))
		})

		It("should accept FourEyes Pending->Semigranted with one decision", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStatePending, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not require decisions for Auto strategy state changes", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateRejected, approvalv1.ApprovalStateRejected, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, approvalv1.ApprovalStateRejected, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not require decisions when state has not changed", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject invalid state transition", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, approvalv1.ApprovalStatePending, oneDecision)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStatePending, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid state transition"))
		})
	})

	Context("FourEyes distinct deciders for Approval", func() {
		var validator ApprovalCustomValidator
		BeforeEach(func() {
			validator = ApprovalCustomValidator{OperatorServiceAccount: "system:serviceaccount:system:controller-manager"}
		})

		makeApproval := func(oldState, newState approvalv1.ApprovalState, decisions []approvalv1.Decision) (*approvalv1.Approval, *approvalv1.Approval) {
			old := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyFourEyes,
					State:    oldState,
				},
			}
			new := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy:  approvalv1.ApprovalStrategyFourEyes,
					State:     newState,
					Decisions: decisions,
				},
			}
			return old, new
		}

		It("should accept Semigranted->Granted with distinct deciders", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject Semigranted->Granted with same decider", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})

		It("should reject Semigranted->Granted with empty email on one decision", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty email"))
		})

		It("should reject Semigranted->Granted with only one decision", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Only one", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least two decisions"))
		})

		It("should not enforce distinct deciders for non-Semigranted->Granted transitions", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Approved", ResultingState: approvalv1.ApprovalStateSemigranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStatePending, approvalv1.ApprovalStateSemigranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		grantedThenExpired := []approvalv1.Decision{
			{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateSemigranted},
			{Name: "Bob", Email: "bob@telekom.de", ResultingState: approvalv1.ApprovalStateGranted},
			{Name: approvalv1.SystemDecisionName, ResultingState: approvalv1.ApprovalStateExpired},
		}

		It("should let one person renew an expired approval (Expired->Granted)", func() {
			decisions := append(append([]approvalv1.Decision{}, grantedThenExpired...),
				approvalv1.Decision{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateGranted})
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject renewing an expired approval without the person's email", func() {
			decisions := append(append([]approvalv1.Decision{}, grantedThenExpired...),
				approvalv1.Decision{Name: "Alice", ResultingState: approvalv1.ApprovalStateGranted})
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty email"))
		})

		It("should let one person resume a suspended approval (Suspended->Granted)", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "bob@telekom.de", ResultingState: approvalv1.ApprovalStateGranted},
				{Name: "Bob", Email: "bob@telekom.de", ResultingState: approvalv1.ApprovalStateSuspended},
				{Name: "Bob", Email: "bob@telekom.de", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject resuming a suspended approval without the person's email", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "bob@telekom.de", ResultingState: approvalv1.ApprovalStateGranted},
				{Name: "Bob", Email: "bob@telekom.de", ResultingState: approvalv1.ApprovalStateSuspended},
				{Name: "Bob", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSuspended, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty email"))
		})

		It("should reject a one-step grant whose last decision does not grant", func() {
			decisions := append(append([]approvalv1.Decision{}, grantedThenExpired...),
				approvalv1.Decision{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateRejected})
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateExpired, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("granting decision"))
		})

		It("should reject completing a re-approval after rejection by the person who semigranted it", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "bob@telekom.de", ResultingState: approvalv1.ApprovalStateRejected},
				{Name: "Carol", Email: "carol@telekom.de", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Carol", Email: "CAROL@telekom.de", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("two distinct deciders"))
		})

		It("should accept completing a re-approval after rejection by a second person", func() {
			decisions := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Bob", Email: "bob@telekom.de", ResultingState: approvalv1.ApprovalStateRejected},
				{Name: "Carol", Email: "carol@telekom.de", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not count a System decision as the first decider on Semigranted->Granted", func() {
			decisions := []approvalv1.Decision{
				{Name: approvalv1.SystemDecisionName, ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj, newObj := makeApproval(approvalv1.ApprovalStateSemigranted, approvalv1.ApprovalStateGranted, decisions)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-empty email"))
		})

		It("should keep Simple Expired->Granted free of email checks", func() {
			decisions := append(append([]approvalv1.Decision{}, grantedThenExpired...),
				approvalv1.Decision{Name: "Alice", ResultingState: approvalv1.ApprovalStateGranted})
			oldObj := &approvalv1.Approval{Spec: approvalv1.ApprovalSpec{Strategy: approvalv1.ApprovalStrategySimple, State: approvalv1.ApprovalStateExpired}}
			newObj := &approvalv1.Approval{Spec: approvalv1.ApprovalSpec{Strategy: approvalv1.ApprovalStrategySimple, State: approvalv1.ApprovalStateGranted, Decisions: decisions}}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should keep Auto Expired->Granted free of decision checks", func() {
			oldObj := &approvalv1.Approval{Spec: approvalv1.ApprovalSpec{Strategy: approvalv1.ApprovalStrategyAuto, State: approvalv1.ApprovalStateExpired}}
			newObj := &approvalv1.Approval{Spec: approvalv1.ApprovalSpec{Strategy: approvalv1.ApprovalStrategyAuto, State: approvalv1.ApprovalStateGranted}}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("stateChanged uses oldObj.Spec.State not Status.LastState", func() {
		var validator ApprovalCustomValidator
		BeforeEach(func() {
			validator = ApprovalCustomValidator{OperatorServiceAccount: "system:serviceaccount:system:controller-manager"}
		})

		It("should detect state change even when Status.LastState is stale", func() {
			// Simulate: controller set LastState=Pending, then user changes state to Granted,
			// but Status.LastState hasn't been updated yet (still Pending).
			// oldObj has Spec.State=Pending (the real old state).
			// newObj has Spec.State=Granted but Status.LastState=Pending (stale, same as Spec.State of old).
			oldObj := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStatePending,
				},
			}
			newObj := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{
						{Name: "Alice", Email: "alice@telekom.de", Comment: "ok", ResultingState: approvalv1.ApprovalStateGranted},
					},
				},
				Status: approvalv1.ApprovalStatus{
					LastState: approvalv1.ApprovalStatePending,
				},
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not flag as state change when only Status.LastState differs", func() {
			// Both oldObj and newObj have Spec.State=Granted (no real change),
			// but newObj.Status.LastState=Pending (stale). Should NOT trigger decision check.
			oldObj := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
				},
			}
			newObj := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
				},
				Status: approvalv1.ApprovalStatus{
					LastState: approvalv1.ApprovalStatePending, // stale
				},
			}
			// No decisions provided. If StateChanged() were used (comparing Status.LastState vs Spec.State),
			// this would incorrectly require a decision. With oldObj comparison, it should pass.
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("defaultDecisionFields via Approval defaulter", func() {
		It("should populate Timestamp and ResultingState on new decisions", func() {
			defaulter := ApprovalCustomDefaulter{}
			before := time.Now()
			a := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategySimple,
					State:    approvalv1.ApprovalStateGranted,
					Decisions: []approvalv1.Decision{
						{Name: "Alice", Email: "alice@telekom.de", Comment: "ok"},
					},
				},
			}

			err := defaulter.Default(context.Background(), a)
			Expect(err).NotTo(HaveOccurred())

			d := a.Spec.Decisions[0]
			Expect(d.ResultingState).To(Equal(approvalv1.ApprovalStateGranted))
			Expect(d.Timestamp).NotTo(BeNil())
			Expect(d.Timestamp.Time).To(BeTemporally(">=", before))
			Expect(d.Timestamp.Time).To(BeTemporally("<=", time.Now()))
		})

		It("should not overwrite existing Timestamp or ResultingState", func() {
			defaulter := ApprovalCustomDefaulter{}
			fixedTime := metav1.NewTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			a := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
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

			err := defaulter.Default(context.Background(), a)
			Expect(err).NotTo(HaveOccurred())

			d := a.Spec.Decisions[0]
			Expect(d.ResultingState).To(Equal(approvalv1.ApprovalStateSemigranted))
			Expect(d.Timestamp.Time).To(Equal(fixedTime.Time))
		})
	})

	Context("on-the-fly FSM validation for Approval (Bug 1+2 fix)", func() {
		var validator ApprovalCustomValidator
		BeforeEach(func() {
			validator = ApprovalCustomValidator{OperatorServiceAccount: "system:serviceaccount:system:controller-manager"}
		})

		makeApproval := func(strategy approvalv1.ApprovalStrategy, specState approvalv1.ApprovalState, decisions []approvalv1.Decision) *approvalv1.Approval {
			return &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
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
			// Simple strategy: Pending -> Semigranted is NOT a valid transition.
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, oneDecision)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateSemigranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid state transition"))
		})

		It("should reject invalid transition regardless of stale Status.AvailableTransitions (Bug 1)", func() {
			// FourEyes: Rejected -> Granted is NOT valid (must go Rejected -> Semigranted first).
			oldObj := makeApproval(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateRejected, oneDecision)
			oldObj.Status.AvailableTransitions = approvalv1.AvailableTransitions{
				{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateGranted}, // stale!
			}
			newObj := makeApproval(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateGranted, oneDecision)
			newObj.Status.AvailableTransitions = approvalv1.AvailableTransitions{
				{Action: approvalv1.ApprovalActionAllow, To: approvalv1.ApprovalStateGranted}, // stale!
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid state transition"))
		})

		It("should allow valid Simple transitions with nil Status", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStatePending, oneDecision)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow valid FourEyes Pending -> Semigranted with nil Status", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStatePending, oneDecision)
			newObj := makeApproval(approvalv1.ApprovalStrategyFourEyes, approvalv1.ApprovalStateSemigranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow valid Approval Suspend transition (Granted -> Suspended)", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, oneDecision)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateSuspended, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow valid Approval Resume transition (Suspended -> Granted)", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateSuspended, oneDecision)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateGranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject Suspended -> Rejected for Simple strategy (no such transition)", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateSuspended, oneDecision)
			newObj := makeApproval(approvalv1.ApprovalStrategySimple, approvalv1.ApprovalStateRejected, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid state transition"))
		})

		It("should allow Auto strategy transitions (Rejected -> Granted)", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateRejected, oneDecision)
			newObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, oneDecision)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow Auto strategy Granted -> Rejected (Deny)", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateRejected, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow Auto strategy Granted -> Suspended (Suspend)", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateSuspended, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow Auto strategy Suspended -> Granted (Resume)", func() {
			oldObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateSuspended, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject Auto strategy invalid transition (Pending -> Granted has no FSM path)", func() {
			// Auto FSM only has Rejected->Granted, Granted->Rejected, Granted->Suspended, Suspended->Granted
			// Pending is not a valid source state in Auto FSM for Approval
			oldObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStatePending, nil)
			newObj := makeApproval(approvalv1.ApprovalStrategyAuto, approvalv1.ApprovalStateGranted, nil)
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid state transition"))
		})
	})

	Context("FourEyes distinct deciders only on Semigranted completion", func() {
		var validator ApprovalCustomValidator
		BeforeEach(func() {
			validator = ApprovalCustomValidator{OperatorServiceAccount: "system:serviceaccount:system:controller-manager"}
		})

		It("should enforce distinct deciders on Semigranted -> Granted (original case)", func() {
			twoSameDeciders := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "First", ResultingState: approvalv1.ApprovalStateSemigranted},
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Second", ResultingState: approvalv1.ApprovalStateGranted},
			}
			oldObj := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyFourEyes,
					State:    approvalv1.ApprovalStateSemigranted,
				},
			}
			newObj := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
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
			oneDecision := []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Approved", ResultingState: approvalv1.ApprovalStateSemigranted},
			}
			oldObj := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy: approvalv1.ApprovalStrategyFourEyes,
					State:    approvalv1.ApprovalStatePending,
				},
			}
			newObj := &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy:  approvalv1.ApprovalStrategyFourEyes,
					State:     approvalv1.ApprovalStateSemigranted,
					Decisions: oneDecision,
				},
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// A scoped Approval whose revocation is lifted is rebound to a newer
	// ApprovalRequest: the step is validated against that request, not the FSM.
	Context("rebinding a scoped Approval to a newer ApprovalRequest", func() {
		const key = "provider"
		revokedAt := metav1.NewTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
		after := metav1.NewTime(revokedAt.Add(time.Hour))
		target := ctypes.TypedObjectRef{
			TypeMeta:  metav1.TypeMeta{APIVersion: "testgroup.cp.ei.telekom.de/v1", Kind: "TestResource"},
			ObjectRef: ctypes.ObjectRef{Name: "listener", Namespace: "default", UID: "target-uid"},
		}
		approve := func(name, email string, state approvalv1.ApprovalState) approvalv1.Decision {
			return approvalv1.Decision{Name: name, Email: email, ResultingState: state, Timestamp: &after}
		}
		fourEyesGrant := []approvalv1.Decision{
			approve("Bob", "bob@example.com", approvalv1.ApprovalStateSemigranted),
			approve("Carol", "carol@example.com", approvalv1.ApprovalStateGranted),
		}

		var (
			request        *approvalv1.ApprovalRequest
			oldObj, newObj *approvalv1.Approval
		)
		BeforeEach(func() {
			request = &approvalv1.ApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "new-request", Namespace: "default", UID: "new-uid"},
				Spec: approvalv1.ApprovalRequestSpec{
					Target:      target,
					ApprovalKey: key,
					Strategy:    approvalv1.ApprovalStrategyFourEyes,
					State:       approvalv1.ApprovalStateGranted,
					Decisions:   slices.Clone(fourEyesGrant),
				},
			}
			oldObj = &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{Name: "approval", Namespace: "default"},
				Spec: approvalv1.ApprovalSpec{
					Target:          target,
					ApprovalKey:     key,
					Strategy:        approvalv1.ApprovalStrategyFourEyes,
					State:           approvalv1.ApprovalStateRejected,
					ApprovedRequest: &ctypes.ObjectRef{Name: "old-request", Namespace: "default", UID: "old-uid"},
					Decisions: []approvalv1.Decision{
						{Name: "Alice", Email: "alice@example.com", ResultingState: approvalv1.ApprovalStateRejected, Timestamp: &revokedAt},
					},
				},
			}
			newObj = oldObj.DeepCopy()
			newObj.Spec.State = approvalv1.ApprovalStateGranted
			newObj.Spec.ApprovedRequest = &ctypes.ObjectRef{Name: "new-request", Namespace: "default", UID: "new-uid"}
			newObj.Spec.Decisions = append(newObj.Spec.Decisions, fourEyesGrant...)
		})
		validate := func() error {
			validator := ApprovalCustomValidator{
				OperatorServiceAccount: "system:serviceaccount:system:controller-manager",
				Reader:                 fake.NewClientBuilder().WithScheme(newWebhookTestScheme()).WithObjects(request).Build(),
			}
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			return err
		}

		It("allows a Rejected FourEyes Approval to be granted for a newer request granted by two people", func() {
			Expect(validate()).To(Succeed())
		})

		It("allows a Semigranted Approval to be granted for a newer Simple request", func() {
			oldObj.Spec.State = approvalv1.ApprovalStateSemigranted
			request.Spec.Strategy = approvalv1.ApprovalStrategySimple
			request.Spec.Decisions = fourEyesGrant[1:]
			newObj.Spec.Strategy = approvalv1.ApprovalStrategySimple
			Expect(validate()).To(Succeed())
		})

		DescribeTable("rejects a rebind the referenced request does not back",
			func(forge func(), message string) {
				forge()
				Expect(validate()).To(MatchError(ContainSubstring(message)))
			},
			Entry("request not granted", func() { request.Spec.State = approvalv1.ApprovalStateSemigranted }, "not granted"),
			Entry("referenced request is terminating", func() {
				request.Finalizers = []string{"test/finalizer"}
				request.DeletionTimestamp = &after
			}, "not granted"),
			Entry("request missing", func() { newObj.Spec.ApprovedRequest.Name = "missing-request" }, "not found"),
			Entry("request recreated", func() { newObj.Spec.ApprovedRequest.UID = "other-uid" }, "UID"),
			Entry("request in another namespace", func() { newObj.Spec.ApprovedRequest.Namespace = "other" }, "namespace"),
			Entry("request for another target", func() { request.Spec.Target.UID = "other-target" }, "scope"),
			Entry("request for another gate", func() { request.Spec.ApprovalKey = "consumer" }, "scope"),
			Entry("request of another strategy", func() { request.Spec.Strategy = approvalv1.ApprovalStrategySimple }, "strategy"),
			Entry("FourEyes request granted by one person", func() {
				request.Spec.Decisions[1].Email = "bob@example.com"
			}, "two distinct deciders"),
			Entry("rebind to a state other than Granted", func() {
				newObj.Spec.State = approvalv1.ApprovalStateSemigranted
				newObj.Spec.Decisions = newObj.Spec.Decisions[:2]
			}, "Granted"),
		)

		It("keeps the FSM for a scoped Approval granted again for the same request", func() {
			newObj.Spec.ApprovedRequest = oldObj.Spec.ApprovedRequest.DeepCopy()
			Expect(validate()).To(MatchError(ContainSubstring("Invalid state transition")))
		})

		It("keeps the FSM for an unscoped Approval", func() {
			oldObj.Spec.ApprovalKey = ""
			newObj.Spec.ApprovalKey = ""
			request.Spec.ApprovalKey = ""
			Expect(validate()).To(MatchError(ContainSubstring("Invalid state transition")))
		})
	})
})

func newWebhookTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	Expect(approvalv1.AddToScheme(s)).To(Succeed())
	return s
}
