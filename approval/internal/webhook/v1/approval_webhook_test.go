// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// controllerContext returns a context that simulates a request from the controller service account.
func controllerContext() context.Context {
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authenticationv1.UserInfo{
				Username: "system:serviceaccount:controlplane-system:approval-controller-manager",
			},
		},
	}
	return admission.NewContextWithRequest(context.Background(), req)
}

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
			validator := ApprovalCustomValidator{}
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
			validator := ApprovalCustomValidator{}
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
	})

	Context("stateChanged uses oldObj.Spec.State not Status.LastState", func() {
		var validator ApprovalCustomValidator

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

	Context("broadened distinct-decider check for Approval (Bug 3 fix)", func() {
		var validator ApprovalCustomValidator

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

	Context("Expire action validation (system-only)", func() {
		var validator ApprovalCustomValidator

		makeApproval := func(specState approvalv1.ApprovalState, decisions []approvalv1.Decision) *approvalv1.Approval {
			return &approvalv1.Approval{
				Spec: approvalv1.ApprovalSpec{
					Strategy:  approvalv1.ApprovalStrategySimple,
					State:     specState,
					Decisions: decisions,
				},
			}
		}

		It("should reject manual transition to EXPIRED without System decision", func() {
			oldObj := makeApproval(approvalv1.ApprovalStateGranted, nil)
			newObj := makeApproval(approvalv1.ApprovalStateExpired, []approvalv1.Decision{
				{Name: "Alice", Email: "alice@telekom.de", Comment: "Manual expire", ResultingState: approvalv1.ApprovalStateExpired},
			})
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Expire action is system-only"))
		})

		It("should reject manual transition to EXPIRED from SUSPENDED without System decision", func() {
			oldObj := makeApproval(approvalv1.ApprovalStateSuspended, nil)
			newObj := makeApproval(approvalv1.ApprovalStateExpired, []approvalv1.Decision{
				{Name: "Bob", Email: "bob@telekom.de", Comment: "Manual expire", ResultingState: approvalv1.ApprovalStateExpired},
			})
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Expire action is system-only"))
		})

		It("should allow controller service account to transition to EXPIRED", func() {
			oldObj := makeApproval(approvalv1.ApprovalStateGranted, nil)
			newObj := makeApproval(approvalv1.ApprovalStateExpired, []approvalv1.Decision{
				{Name: "System", Email: "", Comment: "Automatically expired", ResultingState: approvalv1.ApprovalStateExpired},
			})
			_, err := validator.ValidateUpdate(controllerContext(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow controller service account to transition to EXPIRED from SUSPENDED", func() {
			oldObj := makeApproval(approvalv1.ApprovalStateSuspended, nil)
			newObj := makeApproval(approvalv1.ApprovalStateExpired, []approvalv1.Decision{
				{Name: "System", Email: "", Comment: "Automatically expired", ResultingState: approvalv1.ApprovalStateExpired},
			})
			_, err := validator.ValidateUpdate(controllerContext(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow transition from EXPIRED to GRANTED (re-approval)", func() {
			oldObj := makeApproval(approvalv1.ApprovalStateExpired, []approvalv1.Decision{
				{Name: "System", Email: "", Comment: "Automatically expired", ResultingState: approvalv1.ApprovalStateExpired},
			})
			newObj := makeApproval(approvalv1.ApprovalStateGranted, []approvalv1.Decision{
				{Name: "System", Email: "", Comment: "Automatically expired", ResultingState: approvalv1.ApprovalStateExpired},
				{Name: "Charlie", Email: "charlie@telekom.de", Comment: "Re-approved", ResultingState: approvalv1.ApprovalStateGranted},
			})
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow transition from EXPIRED to REJECTED", func() {
			oldObj := makeApproval(approvalv1.ApprovalStateExpired, []approvalv1.Decision{
				{Name: "System", Email: "", Comment: "Automatically expired", ResultingState: approvalv1.ApprovalStateExpired},
			})
			newObj := makeApproval(approvalv1.ApprovalStateRejected, []approvalv1.Decision{
				{Name: "System", Email: "", Comment: "Automatically expired", ResultingState: approvalv1.ApprovalStateExpired},
				{Name: "Dave", Email: "dave@telekom.de", Comment: "Denied", ResultingState: approvalv1.ApprovalStateRejected},
			})
			_, err := validator.ValidateUpdate(context.Background(), oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
