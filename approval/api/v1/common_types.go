// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

type ApprovalStrategy string

const (
	ApprovalStrategyAuto     ApprovalStrategy = "Auto"
	ApprovalStrategySimple   ApprovalStrategy = "Simple"
	ApprovalStrategyFourEyes ApprovalStrategy = "FourEyes"
)

type ApprovalAction string

const (
	ApprovalActionAllow   ApprovalAction = "Allow"
	ApprovalActionDeny    ApprovalAction = "Deny"
	ApprovalActionSuspend ApprovalAction = "Suspend"
	ApprovalActionResume  ApprovalAction = "Resume"
)

func (a ApprovalAction) String() string {
	return string(a)
}

type ApprovalState string

const (
	ApprovalStatePending     ApprovalState = "Pending"
	ApprovalStateSemigranted ApprovalState = "Semigranted"
	ApprovalStateGranted     ApprovalState = "Granted"
	ApprovalStateRejected    ApprovalState = "Rejected"
	ApprovalStateSuspended   ApprovalState = "Suspended"
	ApprovalStateExpired     ApprovalState = "Expired"
)

func (s ApprovalState) String() string {
	return string(s)
}

type AvailableTransitions []AvailableTransition

func (at AvailableTransitions) HasState(state ApprovalState) bool {
	for _, t := range at {
		if t.To == state {
			return true
		}
	}
	return false
}

type AvailableTransition struct {
	Action ApprovalAction `json:"action"`
	To     ApprovalState  `json:"to"`
}

type Requester struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Reason string `json:"reason"`

	// Properties contains detailed information about the access that was requested
	Properties runtime.RawExtension `json:"properties,omitempty"`
}

func (r *Requester) SetProperties(properties map[string]any) error {
	b, err := json.Marshal(properties)
	if err != nil {
		return errors.Wrap(err, "properties are invalid")
	}
	r.Properties = runtime.RawExtension{Raw: b}
	return nil
}

func (r *Requester) GetProperties() (map[string]any, error) {
	if r.Properties.Raw == nil {
		return nil, nil
	}
	var properties map[string]any
	if err := json.Unmarshal(r.Properties.Raw, &properties); err != nil {
		return nil, errors.Wrap(err, "properties are invalid")
	}
	return properties, nil
}

type Decider struct {
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
	Comment string `json:"comment,omitempty"`
}
