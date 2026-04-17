// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// Authorization defines a permission rule. Supports 3 formats:
// 1. Resource-oriented: resource + permissions (role/actions list)
// 2. Role-oriented: role + permissions (resource/actions list)
// 3. Flat: role + resource + actions directly
//
// +kubebuilder:validation:XValidation:rule="has(self.permissions) || (has(self.resource) && has(self.role) && has(self.actions))", message="Must provide either permissions list or all of (resource, role, actions)"
// +kubebuilder:validation:XValidation:rule="!has(self.permissions) || !has(self.actions)", message="Cannot specify both permissions and actions"
//
// NOTE: Additional validation for nested permissions[] is done in the webhook (rover/internal/webhook/v1/rover_webhook.go)
// rather than CEL due to cost budget constraints. CEL rules with .all() iteration over permissions arrays would exceed
// the Kubernetes validation cost budget by over 40x. The webhook validates that:
// - When resource is set (resource-oriented), all permissions must have non-empty role
// - When role is set (role-oriented), all permissions must have non-empty resource
type Authorization struct {
	// Resource is the resource identifier being protected (used in resource-oriented and flat formats)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=256
	Resource string `json:"resource,omitempty"`

	// Role is the role identifier (used in role-oriented and flat formats)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=256
	Role string `json:"role,omitempty"`

	// Actions lists the allowed actions (used only in flat format)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	Actions []string `json:"actions,omitempty"`

	// Permissions lists role-resource-action tuples (used in resource-oriented and role-oriented formats)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	Permissions []AuthorizationPermission `json:"permissions,omitempty"`
}

// AuthorizationPermission defines a single permission entry within an Authorization
type AuthorizationPermission struct {
	// Role is the role identifier (used when parent Authorization has resource set)
	// +kubebuilder:validation:Optional
	Role string `json:"role,omitempty"`

	// Resource is the resource identifier (used when parent Authorization has role set)
	// +kubebuilder:validation:Optional
	Resource string `json:"resource,omitempty"`

	// Actions lists the allowed actions for this role-resource combination
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	Actions []string `json:"actions"`
}
