// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

type ApprovalStrategy string

const (
	ApprovalStrategyAuto     ApprovalStrategy = "Auto"
	ApprovalStrategySimple   ApprovalStrategy = "Simple"
	ApprovalStrategyFourEyes ApprovalStrategy = "FourEyes"
)

// AutoApprovedComment is the comment added to auto-approved ApprovalRequests.
const AutoApprovedComment = "Auto-approved: The approval strategy does not require manual review."

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
	// TeamName is the name of the team requesting access
	TeamName string `json:"teamName"`

	// TeamEmail is the email address of the team requesting access
	TeamEmail string `json:"teamEmail"`

	// Reason is the reason for requesting access
	Reason string `json:"reason"`

	// ApplicationRef is a reference to the application that is requesting access
	ApplicationRef *ctypes.TypedObjectRef `json:"applicationRef,omitempty"`

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
	// TeamName is the name of the team that decides on the approval request
	TeamName string `json:"teamName,omitempty"`

	// TeamEmail is the email address of the team that decides on the approval request
	TeamEmail string `json:"teamEmail,omitempty"`

	// ApplicationRef is a reference to the application that decides on the approval request
	ApplicationRef *ctypes.TypedObjectRef `json:"applicationRef,omitempty"`
}

type Decision struct {
	// Name of the person making the decision
	Name string `json:"name"`

	// Email of the person making the decision
	Email string `json:"email,omitempty"`

	// Comment provided by the person making the decision
	Comment string `json:"comment,omitempty"`

	// Timestamp of when the decision was made
	// +optional
	Timestamp *metav1.Time `json:"timestamp,omitempty"`

	// ResultingState is the state the resource transitioned to as a result of this decision.
	// Automatically set by the defaulting webhook to match Spec.State when not provided.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Pending;Semigranted;Granted;Rejected;Suspended;Expired
	ResultingState ApprovalState `json:"resultingState"`
}
