// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FileTypeSpec defines the desired state of FileType.
// A FileType is the file-domain registry entry for a file type. It is created in the
// file domain from a rover-domain FileSpecification (1:1) and is the canonical
// resource that FileExposure (1:1) and FileSubscription (1:n) reference via their
// fileTypeRef (mirrors event.EventType).
type FileTypeSpec struct {
	// Description provides a human-readable summary of this file type.
	// +optional
	Description string `json:"description,omitempty"`

	// ExposureRef references the file-domain FileExposure created for this file type (1:1).
	// +optional
	ExposureRef *ctypes.ObjectRef `json:"exposureRef,omitempty"`

	// SubscriptionRefs references the file-domain FileSubscriptions created for this
	// file type (1:n).
	// +optional
	SubscriptionRefs []ctypes.ObjectRef `json:"subscriptionRefs,omitempty"`
}

// FileTypeStatus defines the observed state of FileType.
type FileTypeStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this FileType has been provisioned.
	Active bool `json:"active,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ftype
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Whether this file type is provisioned"
// +kubebuilder:printcolumn:name="CreatedAt",type="date",JSONPath=".metadata.creationTimestamp",description="Creation timestamp"

// FileType is the Schema for the filetypes API.
// It represents a registered file type in the file domain, serving as the canonical
// reference that FileExposure and FileSubscription point to (mirrors event.EventType).
type FileType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileTypeSpec   `json:"spec,omitempty"`
	Status FileTypeStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &FileType{}

func (r *FileType) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *FileType) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// FileTypeList contains a list of FileType.
type FileTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FileType `json:"items"`
}

var _ ctypes.ObjectList = &FileTypeList{}

func (r *FileTypeList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&FileType{}, &FileTypeList{})
}
