// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fsm

import (
	v1 "github.com/telekom/controlplane/approval/api/v1"
)

type Transition struct {
	Action v1.ApprovalAction  `json:"action"`
	Src    []v1.ApprovalState `json:"src"`
	Dst    v1.ApprovalState   `json:"dst"`
}

type Transitions []Transition

type FSM struct {
	Transitions Transitions
}

func (f *FSM) NextState(action v1.ApprovalAction, state v1.ApprovalState) (v1.ApprovalState, bool) {
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

func (f *FSM) AvailableTransitions(state v1.ApprovalState) []v1.AvailableTransition {
	var result []v1.AvailableTransition
	for _, t := range f.Transitions {
		for _, s := range t.Src {
			if s == state {
				result = append(result, v1.AvailableTransition{Action: t.Action, To: t.Dst})
			}
		}
	}
	return result
}

func (f *FSM) CanTransition(action v1.ApprovalAction, state v1.ApprovalState) bool {
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
