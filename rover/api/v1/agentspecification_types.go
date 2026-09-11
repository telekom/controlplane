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

// MakeAgentSpecificationName generates a name for the AgentCard resource based on the BasePath.
// It normalizes the BasePath by removing leading slashes and replacing slashes with hyphens.
func MakeAgentSpecificationName(basePath string) string {
	name := strings.TrimPrefix(basePath, "/")
	return strings.ToLower(strings.ReplaceAll(name, "/", "-"))
}

type AgentSpecificationSpec struct {
	// Specification contains the file ID reference from the file manager
	// +kubebuilder:validation:Required
	Specification string `json:"specification"`

	// Category of the agent, defaults to "other" if not specified
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=other
	Category string `json:"category"`

	// BasePath represents the base path for the agent
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^/.*$`
	// +kubebuilder:validation:MaxLength:=200
	BasePath string `json:"basepath"`

	// Hash is the SHA-256 hash of the specification content
	// +kubebuilder:validation:Required
	Hash string `json:"hash"`

	// Name is a human-readable name for the agent
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Version of the agent specification
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+.*$`
	Version string `json:"version"`

	// Description provides a human-readable summary
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Oauth2Scopes contains the OAuth2 scopes
	// +kubebuilder:validation:Optional
	Oauth2Scopes []string `json:"scopes,omitempty"`
}

type AgentSpecificationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// AgentCard reference
	AgentCard types.ObjectRef `json:"agentCard,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AgentSpecification is the Schema for the agentspecifications API
// +kubebuilder:pruning:PreserveUnknownFields
type AgentSpecification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpecificationSpec   `json:"spec,omitempty"`
	Status AgentSpecificationStatus `json:"status,omitempty"`
}

var _ types.Object = &AgentSpecification{}

func (r *AgentSpecification) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *AgentSpecification) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

//+kubebuilder:object:root=true

type AgentSpecificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentSpecification `json:"items"`
}

var _ types.ObjectList = &AgentSpecificationList{}

func (r *AgentSpecificationList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&AgentSpecification{}, &AgentSpecificationList{})
}
