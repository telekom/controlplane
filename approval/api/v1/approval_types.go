// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:XValidation:rule="(has(self.approvalKey) ? self.approvalKey : '') == (has(oldSelf.approvalKey) ? oldSelf.approvalKey : '')",message="spec.approvalKey is immutable after creation"

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

	// Decisions contains information about who or what changed this approval
	// Only the most recent MaxDecisions entries are retained; older ones are
	// dropped by the mutating webhook.
	// +kubebuilder:default={}
	// +kubebuilder:validation:MaxItems=5
	Decisions []Decision `json:"decisions"`

	// Strategy defines the strategy that was used to approve the request
	// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
	// +kubebuilder:default=Auto
	Strategy ApprovalStrategy `json:"strategy"`

	// State defines the state of the approval
	// +kubebuilder:validation:Enum=Pending;Semigranted;Granted;Rejected;Suspended;Expired
	// +kubebuilder:default=Pending
	State ApprovalState `json:"state"`

	// ApprovedRequest contains the reference to the request that was approved with this approval
	ApprovedRequest *types.ObjectRef `json:"approvedRequest,omitempty"`

	// ApprovalKey identifies an independent decision for the same target.
	// Empty or absent preserves the legacy unscoped approval contract.
	// Its non-empty value cannot be added, removed or changed after creation.
	// +optional
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:Pattern="^([a-z0-9]([-a-z0-9]*[a-z0-9])?)?$"
	ApprovalKey string `json:"approvalKey,omitempty"`
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
	// +kubebuilder:validation:Enum=Pending;Semigranted;Granted;Rejected;Suspended;Expired
	// +kubebuilder:default=Pending
	LastState ApprovalState `json:"lastState,omitempty"`

	// ExpiresAt is the timestamp at which this approval expires.
	// +optional
	// +kubebuilder:printcolumn:name="ExpiresAt",type="date",JSONPath=".status.expiresAt"
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// NotificationRefs is a reference to the notifications that were sent for this approval request.
	// Each notification appears at most once (keyed by namespace and name).
	// +listType=map
	// +listMapKey=namespace
	// +listMapKey=name
	// +optional
	NotificationRefs []types.ObjectRef `json:"notificationRefs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Approval is the Schema for the approvals API
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".spec.state",description="The state of the approval"
// +kubebuilder:printcolumn:name="Strategy",type="string",JSONPath=".spec.strategy",description="The strategy used to approve the request"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
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

// AppendDecision records a decision and keeps the list within MaxDecisions.
// The mutating webhook enforces the same bound server-side; this helper keeps
// the object in its final shape client-side.
func (a *Approval) AppendDecision(d Decision) {
	a.Spec.Decisions = TrimDecisions(append(a.Spec.Decisions, d))
}

var _ types.Object = &Approval{}

func ApprovalName(ownerKind, ownerName string) string {
	return labelutil.NormalizeNameValue(ownerKind + "--" + ownerName)
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
