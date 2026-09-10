// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// IndexFieldSpecInstanceRef is the field index key for Users by spec.instanceRef.
const IndexFieldSpecInstanceRef = "spec.instanceRef"

// UserSpec defines the desired state of User
type UserSpec struct {
	// InstanceRef references the SFTP Instance used by this User.
	// +kubebuilder:validation:Required
	InstanceRef types.ObjectRef `json:"instanceRef"`

	// SSHPublicKeys contains the unique SSH public keys that should be assigned to this User.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:uniqueItems=true
	// +kubebuilder:validation:MinItems=1
	SSHPublicKeys []string `json:"sshPublicKeys"`
}

// UserStatus defines the observed state of User
type UserStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// User is the Schema for the users API
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserSpec   `json:"spec,omitempty"`
	Status UserStatus `json:"status,omitempty"`
}

var _ types.Object = &User{}

func (u *User) GetConditions() []metav1.Condition {
	return u.Status.Conditions
}

func (u *User) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&u.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// UserList contains a list of User
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

var _ types.ObjectList = &UserList{}

func (ul *UserList) GetItems() []types.Object {
	items := make([]types.Object, len(ul.Items))
	for i := range ul.Items {
		items[i] = &ul.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}
