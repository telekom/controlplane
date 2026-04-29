// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Permission defines a single permission entry with role, resource, and actions.
type Permission struct {
	// Role is the role identifier that has access to the resource.
	// +kubebuilder:validation:Required
	Role string `json:"role"`

	// Resource is the resource identifier being protected.
	// +kubebuilder:validation:Required
	Resource string `json:"resource"`

	// Actions lists the allowed actions for this role-resource combination.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Actions []string `json:"actions"`
}

// PermissionSetSpec defines the desired state of PermissionSet.
type PermissionSetSpec struct {
	// Permissions lists all role-resource-action tuples for this application.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Permissions []Permission `json:"permissions"`
}

// PermissionSetStatus defines the observed state of PermissionSet.
type PermissionSetStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// PermissionSet references the external PermissionSet CR created for the external service.
	// +optional
	PermissionSet *ctypes.ObjectRef `json:"permissionSet,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PermissionSet is the Schema for the permissionsets API.
// It represents the internal permission configuration that gets transformed
// into an external PermissionSet consumed by the external permission service.
type PermissionSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PermissionSetSpec   `json:"spec,omitempty"`
	Status PermissionSetStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &PermissionSet{}

func (r *PermissionSet) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *PermissionSet) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// PermissionSetList contains a list of PermissionSet
type PermissionSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PermissionSet `json:"items"`
}

var _ ctypes.ObjectList = &PermissionSetList{}

func (r *PermissionSetList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&PermissionSet{}, &PermissionSetList{})
}
