// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApprovalSpec defines the desired state of Approval
type ApprovalSpec struct {
	// Action defines the action that is requested to be performed on the target object
	// +kubebuilder:default=unknown
	Action string `json:"action"`

	// Target contains the reference to the object that wants to access another object
	Target types.TypedObjectRef `json:"target"`

	// Requester contains the information about the entity that is requesting access
	Requester Requester `json:"requester"`

	// Decider contains the information about the entity that owns the requested object
	Decider Decider `json:"decider,omitempty"`

	// Decisions contains information about people who changed this approval
	Decisions []Decision `json:"decisions,omitempty"`

	// Strategy defines the strategy that was used to approve the request
	// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
	// +kubebuilder:default=Auto
	Strategy ApprovalStrategy `json:"strategy"`

	// State defines the state of the approval
	// +kubebuilder:validation:Enum=Pending;Granted;Rejected;Suspended
	// +kubebuilder:default=Pending
	State ApprovalState `json:"state"`

	// ApprovedRequest contains the reference to the request that was approved with this approval
	ApprovedRequest *types.ObjectRef `json:"approvedRequest,omitempty"`
}

// ApprovalStatus defines the observed state of Approval
type ApprovalStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	AvailableTransitions AvailableTransitions `json:"availableTransitions,omitempty"`

	// LastState defines the last state of the approval
	// +kubebuilder:validation:Enum=Pending;Granted;Rejected;Suspended
	// +kubebuilder:default=Pending
	LastState ApprovalState `json:"lastState,omitempty"`

	// NotificationRef is a reference to the notification that was sent for this approval
	NotificationRef *types.ObjectRef `json:"notificationRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Approval is the Schema for the approvals API
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".spec.state",description="The state of the approval"
// +kubebuilder:printcolumn:name="Strategy",type="string",JSONPath=".spec.strategy",description="The strategy used to approve the request"
type Approval struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApprovalSpec   `json:"spec,omitempty"`
	Status ApprovalStatus `json:"status,omitempty"`
}

func (a *Approval) GetConditions() []metav1.Condition {
	return a.Status.Conditions
}

func (a *Approval) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&a.Status.Conditions, condition)
}

func (a *Approval) StateChanged() bool {
	return a.Status.LastState != a.Spec.State
}

var _ types.Object = &Approval{}

func ApprovalName(ownerKind, ownerName string) string {
	return strings.ToLower(ownerKind) + "--" + ownerName
}

// +kubebuilder:object:root=true

// ApprovalList contains a list of Approval
type ApprovalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Approval `json:"items"`
}

var _ types.ObjectList = &ApprovalList{}

func (al *ApprovalList) GetItems() []types.Object {
	items := make([]types.Object, len(al.Items))
	for i := range al.Items {
		items[i] = &al.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Approval{}, &ApprovalList{})
}
