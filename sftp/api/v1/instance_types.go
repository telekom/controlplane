// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// InstanceSpec defines the desired state of Instance.
type InstanceSpec struct {
	// Description is a human-readable description of this Instance.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// SFTPServiceConfigRef references the SFTPServiceConfig used by this Instance.
	// +kubebuilder:validation:Required
	SFTPServiceConfigRef types.ObjectRef `json:"sftpServiceConfigRef"`
}

// InstanceStatus defines the observed state of Instance.
type InstanceStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Users contains the processing status observed for each User that references this Instance.
	// +listType=map
	// +listMapKey=namespace
	// +listMapKey=name
	// +optional
	Users []InstanceUserStatus `json:"users,omitempty"`
}

// InstanceUserStatus contains the Instance-observed status for a User.
type InstanceUserStatus struct {
	// Namespace is the namespace of the User.
	Namespace string `json:"namespace"`

	// Name is the name of the User.
	Name string `json:"name"`

	// ProcessingCondition is the User Processing condition observed by the Instance reconciliation.
	ProcessingCondition metav1.Condition `json:"processingCondition"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SFTPServiceConfig",type="string",JSONPath=".spec.sftpServiceConfigRef.name"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Instance is the Schema for the instances API.
type Instance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InstanceSpec   `json:"spec,omitempty"`
	Status InstanceStatus `json:"status,omitempty"`
}

var _ types.Object = &Instance{}

func (i *Instance) GetConditions() []metav1.Condition {
	return i.Status.Conditions
}

func (i *Instance) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&i.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// InstanceList contains a list of Instance.
type InstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Instance `json:"items"`
}

var _ types.ObjectList = &InstanceList{}

func (il *InstanceList) GetItems() []types.Object {
	items := make([]types.Object, len(il.Items))
	for i := range il.Items {
		items[i] = &il.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Instance{}, &InstanceList{})
}
