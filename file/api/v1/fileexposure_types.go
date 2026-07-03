// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FileExposureSpec defines the desired state of FileExposure.
// It is created in the file domain from a rover-domain Rover exposure (1:1). The
// derived logical Application is created without an Identity client.
type FileExposureSpec struct {
	// Approval configures how subscriptions to this file type are approved.
	// +kubebuilder:default=Simple
	Approval ApprovalStrategy `json:"approval,omitempty"`

	// Visibility defines who can see and subscribe to this file type.
	// +kubebuilder:default=Enterprise
	Visibility Visibility `json:"visibility,omitempty"`

	// FileTypeRef references the file-domain FileType this exposure belongs to.
	// It must live in the same namespace as this FileExposure.
	// +kubebuilder:validation:Required
	FileTypeRef ctypes.ObjectRef `json:"fileTypeRef"`

	// PublicKeys are the SSH public keys registered for the provider's SFTP user.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	PublicKeys []PublicKey `json:"publicKeys"`

	// ZoneServiceConfigRef references the zone-scoped service configuration
	// (provided by the SFTP/DDS domain) used to reach the backend for this exposure.
	// +optional
	ZoneServiceConfigRef *ctypes.ObjectRef `json:"zoneServiceConfigRef,omitempty"`
}

// FileExposureStatus defines the observed state of FileExposure.
type FileExposureStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this exposure has been provisioned.
	Active bool `json:"active,omitempty"`

	// Subscriptions references the file-domain FileSubscriptions bound to this exposure.
	// +optional
	Subscriptions []ctypes.ObjectRef `json:"subscriptions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Whether this exposure is provisioned"
// +kubebuilder:printcolumn:name="CreatedAt",type="date",JSONPath=".metadata.creationTimestamp",description="Creation timestamp"

// FileExposure is the Schema for the fileexposures API.
// It declares that an application exposes a file type. The derived logical
// Application is created without an Identity client.
type FileExposure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileExposureSpec   `json:"spec,omitempty"`
	Status FileExposureStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &FileExposure{}

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

var _ ctypes.ObjectList = &FileExposureList{}

func (r *FileExposureList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&FileExposure{}, &FileExposureList{})
}
