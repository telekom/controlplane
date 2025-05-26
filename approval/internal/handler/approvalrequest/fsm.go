// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	v1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/fsm"
)

var auto = fsm.Transitions{}

var simple = fsm.Transitions{
	{Action: v1.ApprovalActionAllow, Src: []v1.ApprovalState{v1.ApprovalStatePending}, Dst: v1.ApprovalStateGranted},
	{Action: v1.ApprovalActionDeny, Src: []v1.ApprovalState{v1.ApprovalStatePending}, Dst: v1.ApprovalStateRejected},
}

var fourEyes = fsm.Transitions{
	{Action: v1.ApprovalActionAllow, Src: []v1.ApprovalState{v1.ApprovalStatePending}, Dst: v1.ApprovalStateSemigranted},
	{Action: v1.ApprovalActionDeny, Src: []v1.ApprovalState{v1.ApprovalStatePending, v1.ApprovalStateSemigranted}, Dst: v1.ApprovalStateRejected},
	{Action: v1.ApprovalActionAllow, Src: []v1.ApprovalState{v1.ApprovalStateSemigranted}, Dst: v1.ApprovalStateGranted},
}

var transitionMap = map[v1.ApprovalStrategy]fsm.Transitions{
	v1.ApprovalStrategySimple:   simple,
	v1.ApprovalStrategyFourEyes: fourEyes,
	v1.ApprovalStrategyAuto:     auto,
}

var ApprovalStrategyFSM = map[v1.ApprovalStrategy]*fsm.FSM{
	v1.ApprovalStrategySimple:   {Transitions: transitionMap[v1.ApprovalStrategySimple]},
	v1.ApprovalStrategyFourEyes: {Transitions: transitionMap[v1.ApprovalStrategyFourEyes]},
	v1.ApprovalStrategyAuto:     {Transitions: transitionMap[v1.ApprovalStrategyAuto]},
}
