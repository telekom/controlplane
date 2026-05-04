// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApprovalExpirationSpec defines the desired state of ApprovalExpiration
type ApprovalExpirationSpec struct {
	// Approval is a reference to the parent Approval resource
	Approval types.ObjectRef `json:"approval"`

	// Expiration is the absolute date when the approval expires
	Expiration metav1.Time `json:"expiration"`

	// WeeklyReminder is the date from which weekly reminders start
	WeeklyReminder metav1.Time `json:"weeklyReminder"`

	// DailyReminder is the date from which daily reminders start
	DailyReminder metav1.Time `json:"dailyReminder"`
}

// ApprovalExpirationStatus defines the observed state of ApprovalExpiration
type ApprovalExpirationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// LastReminder is the timestamp when the last reminder was sent
	// +optional
	LastReminder *metav1.Time `json:"lastReminder,omitempty"`

	// LastNotificationRef is a reference to the last sent reminder notification
	// +optional
	LastNotificationRef *types.ObjectRef `json:"lastNotificationRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ApprovalExpiration is the Schema for the approvalexpirations API
// +kubebuilder:printcolumn:name="Approval",type="string",JSONPath=".spec.approval.name",description="The parent Approval"
// +kubebuilder:printcolumn:name="Expiration",type="date",JSONPath=".spec.expiration",description="When the approval expires"
type ApprovalExpiration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApprovalExpirationSpec   `json:"spec,omitempty"`
	Status ApprovalExpirationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApprovalExpirationList contains a list of ApprovalExpiration
type ApprovalExpirationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApprovalExpiration `json:"items"`
}

// GetConditions returns the conditions of the ApprovalExpiration
func (ae *ApprovalExpiration) GetConditions() []metav1.Condition {
	return ae.Status.Conditions
}

// SetCondition sets the condition of the ApprovalExpiration
func (ae *ApprovalExpiration) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&ae.Status.Conditions, condition)
}

func init() {
	SchemeBuilder.Register(&ApprovalExpiration{}, &ApprovalExpirationList{})
}
