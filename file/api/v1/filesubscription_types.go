// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FileSubscriptionSpec defines the desired state of FileSubscription.
// It is created in the file domain from a rover-domain Rover subscription (1:1).
type FileSubscriptionSpec struct {
	// FileType is the file type identifier this subscription belongs to.
	// References the FileType CR via MakeFileTypeName() conversion.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	FileType string `json:"fileType"`

	// Sftp holds the SFTP storage-backend-specific configuration for this subscription.
	// Backend-specific settings live under their own sub-object (e.g. sftp) so that
	// additional storage backends can be added without polluting the spec root.
	// +kubebuilder:validation:Required
	Sftp SftpSubscription `json:"sftp"`
}

// SftpSubscription holds the SFTP storage-backend-specific configuration for a FileSubscription.
type SftpSubscription struct {
	// ClientId identifies the consumer application's client on the SFTP backend.
	// +optional
	ClientId string `json:"clientId,omitempty"`

	// PublicKeys are the SSH public keys registered for the consumer's SFTP user.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	PublicKeys []PublicKey `json:"publicKeys"`
}

// FileSubscriptionStatus defines the observed state of FileSubscription.
type FileSubscriptionStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FileType",type="string",JSONPath=".spec.fileType",description="The file type identifier"
// +kubebuilder:printcolumn:name="CreatedAt",type="date",JSONPath=".metadata.creationTimestamp",description="Creation timestamp"

// FileSubscription is the Schema for the filesubscriptions API.
// It declares that an application consumes a file type.
type FileSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FileSubscriptionSpec   `json:"spec,omitempty"`
	Status FileSubscriptionStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &FileSubscription{}

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

var _ ctypes.ObjectList = &FileSubscriptionList{}

func (r *FileSubscriptionList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&FileSubscription{}, &FileSubscriptionList{})
}
