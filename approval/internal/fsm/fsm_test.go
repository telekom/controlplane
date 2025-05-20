// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fsm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/telekom/controlplane/approval/api/v1"
)

var _ = Describe("Approval Finite State Machine", func() {

	exampleTransitions := Transitions{
		{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStatePending}, Dst: ApprovalStateSemigranted},
		{Action: ApprovalActionDeny, Src: []ApprovalState{ApprovalStatePending, ApprovalStateGranted, ApprovalStateSemigranted}, Dst: ApprovalStateRejected},
		{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStateSemigranted}, Dst: ApprovalStateGranted},
	}

	It("should return the next state", func() {
		fsm := FSM{
			Transitions: exampleTransitions,
		}

		By("returning the next state")
		nextState, _ := fsm.NextState(ApprovalActionAllow, ApprovalStatePending)
		Expect(nextState).To(Equal(ApprovalStateSemigranted))

		nextState, _ = fsm.NextState(ApprovalActionDeny, ApprovalStatePending)
		Expect(nextState).To(Equal(ApprovalStateRejected))

		nextState, _ = fsm.NextState(ApprovalActionDeny, ApprovalStateGranted)
		Expect(nextState).To(Equal(ApprovalStateRejected))

		nextState, _ = fsm.NextState(ApprovalActionAllow, ApprovalStateSemigranted)
		Expect(nextState).To(Equal(ApprovalStateGranted))

		By("returning the same state if the transition is not possible")
		nextState, ok := fsm.NextState(ApprovalActionAllow, ApprovalStateGranted)
		Expect(nextState).To(Equal(ApprovalStateGranted))
		Expect(ok).To(BeFalse())
	})

	It("should return the available transitions", func() {
		fsm := FSM{
			Transitions: exampleTransitions,
		}

		By("returning the available transitions")
		transitions := fsm.AvailableTransitions(ApprovalStatePending)
		Expect(transitions).To(HaveLen(2))
		Expect(transitions).To(ConsistOf(
			AvailableTransition{Action: ApprovalActionAllow, To: ApprovalStateSemigranted},
			AvailableTransition{Action: ApprovalActionDeny, To: ApprovalStateRejected},
		))
	})

	It("should return if a transition is possible", func() {
		fsm := FSM{
			Transitions: exampleTransitions,
		}

		By("returning if a transition is possible")
		Expect(fsm.CanTransition(ApprovalActionAllow, ApprovalStatePending)).To(BeTrue())
		Expect(fsm.CanTransition(ApprovalActionDeny, ApprovalStatePending)).To(BeTrue())
		Expect(fsm.CanTransition(ApprovalActionDeny, ApprovalStateGranted)).To(BeTrue())
		Expect(fsm.CanTransition(ApprovalActionAllow, ApprovalStateGranted)).To(BeFalse())
	})

})
