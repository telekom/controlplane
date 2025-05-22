// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	. "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/fsm"
)

var auto = fsm.Transitions{
	{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStateRejected}, Dst: ApprovalStateGranted},
	{Action: ApprovalActionDeny, Src: []ApprovalState{ApprovalStateGranted}, Dst: ApprovalStateRejected},
	{Action: ApprovalActionSuspend, Src: []ApprovalState{ApprovalStateGranted}, Dst: ApprovalStateSuspended},
	{Action: ApprovalActionResume, Src: []ApprovalState{ApprovalStateSuspended}, Dst: ApprovalStateGranted},
}

var simple = fsm.Transitions{
	{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStatePending, ApprovalStateRejected}, Dst: ApprovalStateGranted},
	{Action: ApprovalActionDeny, Src: []ApprovalState{ApprovalStatePending, ApprovalStateGranted}, Dst: ApprovalStateRejected},
	{Action: ApprovalActionSuspend, Src: []ApprovalState{ApprovalStateGranted}, Dst: ApprovalStateSuspended},
	{Action: ApprovalActionResume, Src: []ApprovalState{ApprovalStateSuspended}, Dst: ApprovalStateGranted},
}

var fourEyes = fsm.Transitions{
	{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStatePending, ApprovalStateRejected}, Dst: ApprovalStateSemigranted},
	{Action: ApprovalActionDeny, Src: []ApprovalState{ApprovalStatePending, ApprovalStateGranted, ApprovalStateSemigranted}, Dst: ApprovalStateRejected},
	{Action: ApprovalActionAllow, Src: []ApprovalState{ApprovalStateSemigranted}, Dst: ApprovalStateGranted},
	{Action: ApprovalActionSuspend, Src: []ApprovalState{ApprovalStateGranted}, Dst: ApprovalStateSuspended},
	{Action: ApprovalActionResume, Src: []ApprovalState{ApprovalStateSuspended}, Dst: ApprovalStateGranted},
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
