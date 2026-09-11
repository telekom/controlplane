// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Permission defines a single permission entry with role, resource, and actions.
// This structure is consumed by the external permission service.
type Permission struct {
	// Role is the role identifier that has access to the resource.
	Role string `json:"role"`

	// Resource is the resource identifier being protected.
	Resource string `json:"resource"`

	// Actions lists the allowed actions for this role-resource combination.
	Actions []string `json:"actions"`
}

// PermissionSetSpec defines the desired state of PermissionSet.
type PermissionSetSpec struct {
	// Permissions lists all role-resource-action tuples for this application.
	Permissions []Permission `json:"permissions"`
}

// +kubebuilder:object:root=true

// PermissionSet is the Schema for the permissionsets API consumed by the external permission service.
// This CRD is created by the internal permission operator and read by the external service.
// It has no status subresource as no operator reconciles it.
type PermissionSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PermissionSetSpec `json:"spec,omitempty"`
}

var _ ctypes.Object = &PermissionSet{}

// GetConditions returns nil as the external PermissionSet has no status conditions.
func (r *PermissionSet) GetConditions() []metav1.Condition {
	return nil
}

// SetCondition is a no-op and returns false as the external PermissionSet has no status conditions.
func (r *PermissionSet) SetCondition(condition metav1.Condition) bool {
	return false
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
