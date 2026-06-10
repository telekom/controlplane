// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/telekom/controlplane/common/pkg/reminder"
	"github.com/telekom/controlplane/common/pkg/types"
)

// ApprovalExpirationSpec defines the desired state of ApprovalExpiration.
type ApprovalExpirationSpec struct {
	// Approval is a reference to the parent Approval resource.
	Approval types.ObjectRef `json:"approval"`

	// Expiration is the timestamp at which the approval expires.
	Expiration metav1.Time `json:"expiration"`

	// Thresholds defines when reminder notifications should be sent before expiration.
	// +optional
	Thresholds []reminder.Threshold `json:"thresholds,omitempty"`
}

// ApprovalExpirationStatus defines the observed state of ApprovalExpiration.
type ApprovalExpirationStatus struct {
	// Conditions represent the current state of the ApprovalExpiration.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// SentReminders tracks which reminder notifications have already been sent.
	// +optional
	SentReminders []reminder.SentReminder `json:"sentReminders,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Approval",type="string",JSONPath=".spec.approval.name",description="The parent Approval"
// +kubebuilder:printcolumn:name="Expiration",type="date",JSONPath=".spec.expiration",description="When the approval expires"

// ApprovalExpiration is the Schema for the approvalexpirations API.
type ApprovalExpiration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApprovalExpirationSpec   `json:"spec,omitempty"`
	Status ApprovalExpirationStatus `json:"status,omitempty"`
}

func (ae *ApprovalExpiration) GetConditions() []metav1.Condition {
	return ae.Status.Conditions
}

func (ae *ApprovalExpiration) SetCondition(c metav1.Condition) bool {
	return meta.SetStatusCondition(&ae.Status.Conditions, c)
}

// +kubebuilder:object:root=true

// ApprovalExpirationList contains a list of ApprovalExpiration.
type ApprovalExpirationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApprovalExpiration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApprovalExpiration{}, &ApprovalExpirationList{})
}
