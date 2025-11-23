// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/hash"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApprovalRequestSpec defines the desired state of ApprovalRequest
type ApprovalRequestSpec struct {
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
	// +kubebuilder:validation:Enum=Pending;Granted;Semigranted;Rejected
	// +kubebuilder:default=Pending
	State ApprovalState `json:"state"`
}

// ApprovalRequestStatus defines the observed state of ApprovalRequest
type ApprovalRequestStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	Approval types.ObjectRef `json:"approval"`

	AvailableTransitions AvailableTransitions `json:"availableTransitions,omitempty"`

	// LastState defines the last state of the approval request
	// +kubebuilder:validation:Enum=Pending;Granted;Semigranted;Rejected
	// +kubebuilder:default=Pending
	LastState ApprovalState `json:"lastState,omitempty"`

	// NotificationRefs is a reference to the notifications that were sent for this approval request
	NotificationRefs []types.ObjectRef `json:"notificationRefs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ApprovalRequest is the Schema for the approvalrequests API
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".spec.state",description="The state of the approval"
// +kubebuilder:printcolumn:name="Strategy",type="string",JSONPath=".spec.strategy",description="The strategy used to approve the request"
type ApprovalRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApprovalRequestSpec   `json:"spec,omitempty"`
	Status ApprovalRequestStatus `json:"status,omitempty"`
}

var _ types.Object = &ApprovalRequest{}

func (ar *ApprovalRequest) GetConditions() []metav1.Condition {
	return ar.Status.Conditions
}

func (ar *ApprovalRequest) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&ar.Status.Conditions, condition)
}

func (ar *ApprovalRequest) StateChanged() bool {
	return ar.Status.LastState != ar.Spec.State
}

// NewApprovalRequest returns a new ApprovalRequest object.
// The name of the ApprovalRequest is generated from the owner and its hash(spec) to make it unique
func NewApprovalRequest(owner types.Object, hashValue any) *ApprovalRequest {
	return &ApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ApprovalRequestName(owner, hashValue),
			Namespace: owner.GetNamespace(),
		},
		Spec: ApprovalRequestSpec{},
	}
}

func ApprovalRequestName(owner types.Object, hashValue any) string {
	return owner.GetName() + "--" + hash.ComputeHash(&hashValue, nil)
}

// +kubebuilder:object:root=true

// ApprovalRequestList contains a list of ApprovalRequest
type ApprovalRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApprovalRequest `json:"items"`
}

var _ types.ObjectList = &ApprovalRequestList{}

func (ar *ApprovalRequestList) GetItems() []types.Object {
	items := make([]types.Object, len(ar.Items))
	for i := range ar.Items {
		items[i] = &ar.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ApprovalRequest{}, &ApprovalRequestList{})
}
