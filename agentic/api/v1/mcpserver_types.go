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

// MakeMcpServerName generates a Kubernetes resource name from a basePath.
// It strips leading slashes and replaces "/" with "-" (e.g. "/mcp/weather/v1" -> "mcp-weather-v1").
func MakeMcpServerName(basePath string) string {
	name := strings.TrimPrefix(basePath, "/")
	return strings.ToLower(strings.ReplaceAll(name, "/", "-"))
}

// McpServerSpec defines the desired state of McpServer.
type McpServerSpec struct {
	// BasePath is the base path of the MCP server endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^/[a-z0-9-/]+$`
	BasePath string `json:"basePath"`

	// Version of the MCP server specification (e.g. "1.0.0").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+.*$`
	Version string `json:"version"`

	// Name is a human-readable name for the MCP server.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Description provides a human-readable summary of this MCP server.
	// +optional
	Description string `json:"description,omitempty"`

	// Specification contains the file ID reference from the file manager for
	// the MCP server specification YAML.
	// +optional
	Specification string `json:"specification,omitempty"`

	// Oauth2Scopes contains the OAuth2 scopes extracted from the MCP specification.
	// Subscriptions and exposures that declare scopes are validated against this list.
	// +kubebuilder:validation:Optional
	Oauth2Scopes []string `json:"scopes,omitempty"`
}

// McpServerStatus defines the observed state of McpServer.
type McpServerStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this McpServer is the active singleton for its basePath.
	Active bool `json:"active"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BasePath",type="string",JSONPath=".spec.basePath",description="The MCP server base path"
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Whether this server registration is active"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// McpServer is the Schema for the mcpservers API.
// It represents a registered MCP server definition, serving as the
// canonical reference that McpExposure and McpSubscription point to.
type McpServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   McpServerSpec   `json:"spec,omitempty"`
	Status McpServerStatus `json:"status,omitempty"`
}

var _ types.Object = &McpServer{}

func (r *McpServer) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *McpServer) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// McpServerList contains a list of McpServer
type McpServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []McpServer `json:"items"`
}

var _ types.ObjectList = &McpServerList{}

func (r *McpServerList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&McpServer{}, &McpServerList{})
}
