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

// MakeAgentCardName generates a Kubernetes resource name from a basePath.
// It strips leading slashes and replaces "/" with "-" (e.g. "/agent/assistant/v1" -> "agent-assistant-v1").
func MakeAgentCardName(basePath string) string {
	name := strings.TrimPrefix(basePath, "/")
	return strings.ToLower(strings.ReplaceAll(name, "/", "-"))
}

// AgentCardSpec defines the desired state of AgentCard.
type AgentCardSpec struct {
	// BasePath is the base path of the A2A agent endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^/.*$`
	BasePath string `json:"basePath"`

	// Version of the agent specification (e.g. "1.0.0").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+.*$`
	Version string `json:"version"`

	// Name is a human-readable name for the A2A agent.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Description provides a human-readable summary of this A2A agent.
	// +optional
	Description string `json:"description,omitempty"`

	// Specification contains the file ID reference from the file manager for
	// the agent card JSON.
	// +optional
	Specification string `json:"specification,omitempty"`

	// Category of the agent (e.g. "assistant", "tool", "other").
	// +kubebuilder:validation:Optional
	Category string `json:"category,omitempty"`

	// Oauth2Scopes contains the OAuth2 scopes extracted from the agent specification.
	// Subscriptions and exposures that declare scopes are validated against this list.
	// +kubebuilder:validation:Optional
	Oauth2Scopes []string `json:"scopes,omitempty"`
}

// AgentCardStatus defines the observed state of AgentCard.
type AgentCardStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this AgentCard is the active singleton for its basePath.
	Active bool `json:"active"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BasePath",type="string",JSONPath=".spec.basePath",description="The agent base path"
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Whether this agent registration is active"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AgentCard is the Schema for the agentcards API.
// It represents a registered A2A agent definition, serving as the
// canonical reference that AgenticExposure and AgenticSubscription point to
// when the variant is AGENT.
type AgentCard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentCardSpec   `json:"spec,omitempty"`
	Status AgentCardStatus `json:"status,omitempty"`
}

var _ types.Object = &AgentCard{}

func (r *AgentCard) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *AgentCard) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// AgentCardList contains a list of AgentCard
type AgentCardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentCard `json:"items"`
}

var _ types.ObjectList = &AgentCardList{}

func (r *AgentCardList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&AgentCard{}, &AgentCardList{})
}
