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

// MakeAgenticServerName generates a Kubernetes resource name from a basePath.
// It strips leading slashes and replaces "/" with "-" (e.g. "/mcp/weather/v1" -> "mcp-weather-v1").
func MakeAgenticServerName(basePath string) string {
	name := strings.TrimPrefix(basePath, "/")
	return strings.ToLower(strings.ReplaceAll(name, "/", "-"))
}

// AgenticServerSpec defines the desired state of AgenticServer.
type AgenticServerSpec struct {
	// BasePath is the base path of the MCP server endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^/.*$`
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

	// Category of the MCP server (e.g. "g-api", "m-api", "other").
	// +kubebuilder:validation:Optional
	Category string `json:"category,omitempty"`

	// Oauth2Scopes contains the OAuth2 scopes extracted from the MCP specification.
	// Subscriptions and exposures that declare scopes are validated against this list.
	// +kubebuilder:validation:Optional
	Oauth2Scopes []string `json:"scopes,omitempty"`
}

// AgenticServerStatus defines the observed state of AgenticServer.
type AgenticServerStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this AgenticServer is the active singleton for its basePath.
	Active bool `json:"active"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BasePath",type="string",JSONPath=".spec.basePath",description="The MCP server base path"
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Whether this server registration is active"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AgenticServer is the Schema for the agenticservers API.
// It represents a registered MCP server definition, serving as the
// canonical reference that AgenticExposure and AgenticSubscription point to.
type AgenticServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgenticServerSpec   `json:"spec,omitempty"`
	Status AgenticServerStatus `json:"status,omitempty"`
}

var _ types.Object = &AgenticServer{}

func (r *AgenticServer) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *AgenticServer) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// AgenticServerList contains a list of AgenticServer
type AgenticServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgenticServer `json:"items"`
}

var _ types.ObjectList = &AgenticServerList{}

func (r *AgenticServerList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&AgenticServer{}, &AgenticServerList{})
}
