// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeFileSpecificationName generates a name for the FileType resource based on
// the file type identifier of the FileSpecification (its metadata name).
// MakeEventSpecificationName / MakeName (api).
func MakeFileSpecificationName(fileSpec *FileSpecification) string {
	return strings.ToLower(strings.ReplaceAll(fileSpec.Name, ".", "-"))
}

// FileStorageType selects the file-transfer backend used to store/exchange files
// for a file type. Currently only SFTP is supported
// +kubebuilder:validation:Enum=sftp
type FileStorageType string

const (
	// FileStorageTypeSFTP indicates the file type is handled via the SFTP backend
	FileStorageTypeSFTP FileStorageType = "sftp"
)

func (t FileStorageType) String() string {
	return string(t)
}

// FileSpecificationSpec defines the desired state of FileSpecification.
// It mirrors the internal Rover-domain form from spec_dcp: only description and the
// backend selector are stored; the file type identifier lives in metadata.name.
type FileSpecificationSpec struct {
	// Description provides a human-readable summary of this file type.
	// +optional
	Description string `json:"description,omitempty"`

	// Specification contains the file ID reference from the file manager for the
	// optional document that describes this file type.
	// +optional
	Specification string `json:"specification,omitempty"`

	// StorageType selects the file-transfer backend.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=sftp
	StorageType FileStorageType `json:"storageType,omitempty"`
}

// FileSpecificationStatus defines the observed state of FileSpecification.
type FileSpecificationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// FileType references the file-domain FileType created from this specification.
	// It is populated by the FileSpecification reconciler
	// (rover/internal/controller/filespecification_controller.go), mirroring how
	// ApiSpecification creates Api and EventSpecification creates EventType.
	FileType types.ObjectRef `json:"fileType,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// FileSpecification is the Schema for the filespecifications API.
// It defines a file type's metadata and creates the corresponding file-domain
// FileType, analogous to how ApiSpecification creates Api resources and
// EventSpecification creates EventType resources.
type FileSpecification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileSpecificationSpec   `json:"spec,omitempty"`
	Status FileSpecificationStatus `json:"status,omitempty"`
}

var _ types.Object = &FileSpecification{}

func (r *FileSpecification) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *FileSpecification) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

type FileSpecificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FileSpecification `json:"items"`
}

var _ types.ObjectList = &FileSpecificationList{}

func (r *FileSpecificationList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&FileSpecification{}, &FileSpecificationList{})
}
