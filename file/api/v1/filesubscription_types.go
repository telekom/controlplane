// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// FileSubscriptionSpec defines a consumer-side subscription to a file type.
type FileSubscriptionSpec struct {
	// FileType references the FileType to subscribe to.
	// +kubebuilder:validation:Required
	FileType string `json:"fileType"`

	// Zone identifies the zone where the subscriber is located.
	// +kubebuilder:validation:Required
	Zone *types.ObjectRef `json:"zone,omitempty"`

	// SFTP configures consumer-side SFTP access for this file subscription.
	// +kubebuilder:validation:Optional
	SFTP *FileSFTP `json:"sftp,omitempty"`
}

// FileSubscriptionStatus defines the observed state of FileSubscription.
type FileSubscriptionStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Approval references the Approval CR managing subscription approval.
	// +optional
	Approval *types.ObjectRef `json:"approval,omitempty"`

	// ApprovalRequest references the ApprovalRequest CR for this subscription.
	// +optional
	ApprovalRequest *types.ObjectRef `json:"approvalRequest,omitempty"`

	// FileTypeRef references the subscribed FileType.
	// +optional
	FileTypeRef *types.ObjectRef `json:"fileTypeRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FileType",type="string",JSONPath=".spec.fileType"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FileSubscription is the Schema for the filesubscriptions API.
type FileSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileSubscriptionSpec   `json:"spec,omitempty"`
	Status FileSubscriptionStatus `json:"status,omitempty"`
}

var _ types.Object = &FileSubscription{}

func (r *FileSubscription) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *FileSubscription) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// FileSubscriptionList contains a list of FileSubscription.
type FileSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FileSubscription `json:"items"`
}

var _ types.ObjectList = &FileSubscriptionList{}

func (r *FileSubscriptionList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&FileSubscription{}, &FileSubscriptionList{})
}
