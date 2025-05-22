// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fsm

import (
	. "github.com/telekom/controlplane/approval/api/v1"
)

type Transition struct {
	Action ApprovalAction  `json:"action"`
	Src    []ApprovalState `json:"src"`
	Dst    ApprovalState   `json:"dst"`
}

type Transitions []Transition

type FSM struct {
	Transitions Transitions
}

func (f *FSM) NextState(action ApprovalAction, state ApprovalState) (ApprovalState, bool) {
	for _, t := range f.Transitions {
		if t.Action == action {
			for _, s := range t.Src {
				if s == state {
					return t.Dst, true
				}
			}
		}
	}
	return state, false
}

func (f *FSM) AvailableTransitions(state ApprovalState) []AvailableTransition {
	var result []AvailableTransition
	for _, t := range f.Transitions {
		for _, s := range t.Src {
			if s == state {
				result = append(result, AvailableTransition{Action: t.Action, To: t.Dst})
			}
		}
	}
	return result
}

func (f *FSM) CanTransition(action ApprovalAction, state ApprovalState) bool {
	for _, t := range f.Transitions {
		if t.Action == action {
			for _, s := range t.Src {
				if s == state {
					return true
				}
			}
		}
	}
	return false
}
