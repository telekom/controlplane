// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// FileExposureSpec defines a provider-side file exposure.
type FileExposureSpec struct {
	// Provider optionally identifies the providing application.
	// +kubebuilder:validation:Optional
	Provider string `json:"provider,omitempty"`

	// +kubebuilder:validation:Required
	FileType string `json:"fileType"`

	// Zone identifies the zone where this file exposure is provided.
	// +kubebuilder:validation:Required
	Zone *types.ObjectRef `json:"zone,omitempty"`

	// SFTP configures provider-side SFTP access for this file exposure.
	// +kubebuilder:validation:Optional
	SFTP *FileSFTP `json:"sftp,omitempty"`

	// Visibility defines who can subscribe to this file exposure.
	// +kubebuilder:default=Enterprise
	Visibility Visibility `json:"visibility"`

	// Approval configures how subscriptions to this file exposure are approved.
	// +kubebuilder:validation:Optional
	Approval Approval `json:"approval,omitempty"`
}

// FileExposureStatus defines the observed state of FileExposure.
type FileExposureStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// FileTypeRef references the FileType this exposure provides.
	// +optional
	FileTypeRef *types.ObjectRef `json:"fileTypeRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FileType",type="string",JSONPath=".spec.fileType"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FileExposure is the Schema for the fileexposures API.
type FileExposure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileExposureSpec   `json:"spec,omitempty"`
	Status FileExposureStatus `json:"status,omitempty"`
}

var _ types.Object = &FileExposure{}

func (r *FileExposure) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *FileExposure) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// FileExposureList contains a list of FileExposure.
type FileExposureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FileExposure `json:"items"`
}

var _ types.ObjectList = &FileExposureList{}

func (r *FileExposureList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&FileExposure{}, &FileExposureList{})
}
