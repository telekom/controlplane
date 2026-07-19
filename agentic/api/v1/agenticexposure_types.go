// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgenticExposureSpec defines the desired state of AgenticExposure.
type AgenticExposureSpec struct {
	// BasePath references the AgenticServer via its basePath.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^/.*$`
	BasePath string `json:"basePath"`

	// Upstreams define the backend MCP server targets.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Upstreams []Upstream `json:"upstreams"`

	// Visibility defines who can see and subscribe to this MCP server.
	// +kubebuilder:default=Enterprise
	Visibility Visibility `json:"visibility"`

	// Approval configures how subscriptions to this MCP server are approved.
	Approval Approval `json:"approval"`

	// Zone references the Zone CR where this MCP server is exposed.
	Zone ctypes.ObjectRef `json:"zone"`

	// Provider identifies the providing application.
	Provider ctypes.ObjectRef `json:"provider"`

	// Variant defines the exposure variant (MCP or TELECONTEXTMCP).
	// +kubebuilder:default=MCP
	Variant AgenticVariant `json:"variant"`

	// Security configures optional security settings for the route.
	// +optional
	Security *Security `json:"security,omitempty"`

	// Traffic configures traffic management for the route.
	// +optional
	Traffic Traffic `json:"traffic"`

	// Transformation configures request/response transformations for the route.
	// +optional
	Transformation *Transformation `json:"transformation,omitempty"`
}

// AgenticExposureStatus defines the observed state of AgenticExposure.
type AgenticExposureStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this AgenticExposure is the active one for its basePath.
	Active bool `json:"active"`

	// Route references the primary gateway Route CR created for this exposure.
	// +optional
	Route *ctypes.ObjectRef `json:"route,omitempty"`

	// ProxyRoutes references proxy gateway Route CRs for cross-zone MCP delivery.
	// +optional
	ProxyRoutes []ctypes.ObjectRef `json:"proxyRoutes,omitempty"`

}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BasePath",type="string",JSONPath=".spec.basePath",description="The MCP server base path"
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Whether this exposure is active"
// +kubebuilder:printcolumn:name="Variant",type="string",JSONPath=".spec.variant",description="The exposure variant"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AgenticExposure is the Schema for the agenticexposures API.
// It represents a declaration that an application exposes an MCP server,
// making it available for subscription by other applications via the AI Gateway.
type AgenticExposure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgenticExposureSpec   `json:"spec,omitempty"`
	Status AgenticExposureStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &AgenticExposure{}

func (r *AgenticExposure) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *AgenticExposure) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

func (r *AgenticExposure) HasM2M() bool {
	return r.Spec.Security != nil && r.Spec.Security.M2M != nil
}

func (r *AgenticExposure) HasExternalIdp() bool {
	if !r.HasM2M() {
		return false
	}
	return r.Spec.Security.M2M.ExternalIDP != nil
}

// +kubebuilder:object:root=true

// AgenticExposureList contains a list of AgenticExposure
type AgenticExposureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgenticExposure `json:"items"`
}

var _ ctypes.ObjectList = &AgenticExposureList{}

func (r *AgenticExposureList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&AgenticExposure{}, &AgenticExposureList{})
}
