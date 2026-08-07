// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FileTypeLabelKey is the label used to associate FileExposure/FileSubscription
// resources with their FileType
var FileTypeLabelKey = config.BuildLabelKey("filetype")

// MakeFileTypeName generates a Kubernetes resource name from a file type identifier.
func MakeFileTypeName(fileType string) string {
	return strings.ToLower(strings.ReplaceAll(fileType, ".", "-"))
}

// FileTypeSpec defines the desired state of FileType.
type FileTypeSpec struct {
	// Type is the dot-separated file type identifier (e.g. "de.telekom.eni.invoices.v1").
	// Used to generate the resource name via MakeFileTypeName() conversion.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(\.[a-z0-9]+)*$`
	Type string `json:"type"`

	// Description provides a human-readable summary of this file type.
	// +optional
	Description string `json:"description,omitempty"`

	// Specification contains the file ID reference from the file manager for the
	// optional document that describes this file type.
	// +optional
	Specification string `json:"specification,omitempty"`
}

// FileTypeStatus defines the observed state of FileType.
type FileTypeStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this FileType is the active singleton for its file
	// type identifier. When multiple FileTypes exist for the same identifier, only
	// the oldest non-deleted one is active.
	Active bool `json:"active,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ftype
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="The file type identifier"
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Indicates if this FileType is the active singleton"
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
