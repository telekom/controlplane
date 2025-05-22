// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalrequest

import (
	. "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/fsm"
)

var auto = fsm.Transitions{}

var simple = fsm.Transitions{
	{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStatePending}, Dst: ApprovalStateGranted},
	{Action: ApprovalActionDeny, Src: []ApprovalState{ApprovalStatePending}, Dst: ApprovalStateRejected},
}

var fourEyes = fsm.Transitions{
	{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStatePending}, Dst: ApprovalStateSemigranted},
	{Action: ApprovalActionDeny, Src: []ApprovalState{ApprovalStatePending, ApprovalStateSemigranted}, Dst: ApprovalStateRejected},
	{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStateSemigranted}, Dst: ApprovalStateGranted},
}

var transitionMap = map[ApprovalStrategy]fsm.Transitions{
	ApprovalStrategySimple:   simple,
	ApprovalStrategyFourEyes: fourEyes,
	ApprovalStrategyAuto:     auto,
}

var ApprovalStrategyFSM = map[ApprovalStrategy]*fsm.FSM{
	ApprovalStrategySimple:   {Transitions: transitionMap[ApprovalStrategySimple]},
	ApprovalStrategyFourEyes: {Transitions: transitionMap[ApprovalStrategyFourEyes]},
	ApprovalStrategyAuto:     {Transitions: transitionMap[ApprovalStrategyAuto]},
}
