// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// FileTypeSpec defines a logical file type that can be exposed and subscribed to.
type FileTypeSpec struct {
	// Description is a human-readable description of this file type.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`
}

// FileTypeStatus defines the observed state of FileType.
type FileTypeStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// FileExposureRef references the active FileExposure for this file type.
	// +optional
	FileExposureRef *types.ObjectRef `json:"fileExposureRef,omitempty"`

	// SFTPInstance references the projected SFTP instance for this file type.
	// +optional
	SFTPInstance *types.ObjectRef `json:"sftpInstance,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FileType is the Schema for the filetypes API.
type FileType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileTypeSpec   `json:"spec,omitempty"`
	Status FileTypeStatus `json:"status,omitempty"`
}

var _ types.Object = &FileType{}

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

var _ types.ObjectList = &FileTypeList{}

func (r *FileTypeList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&FileType{}, &FileTypeList{})
}
