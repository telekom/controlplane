// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// McpExposureSpec defines the desired state of McpExposure.
type McpExposureSpec struct {
	// BasePath references the McpServer via its basePath.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^/[a-z0-9-/]+$`
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
	Variant McpVariant `json:"variant"`

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

// McpExposureStatus defines the observed state of McpExposure.
type McpExposureStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this McpExposure is the active one for its basePath.
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
// +kubebuilder:printcolumn:name="CreatedAt",type="date",JSONPath=".metadata.creationTimestamp",description="Creation timestamp"

// McpExposure is the Schema for the mcpexposures API.
// It represents a declaration that an application exposes an MCP server,
// making it available for subscription by other applications via the AI Gateway.
type McpExposure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   McpExposureSpec   `json:"spec,omitempty"`
	Status McpExposureStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &McpExposure{}

func (r *McpExposure) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *McpExposure) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// McpExposureList contains a list of McpExposure
type McpExposureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []McpExposure `json:"items"`
}

var _ ctypes.ObjectList = &McpExposureList{}

func (r *McpExposureList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&McpExposure{}, &McpExposureList{})
}
