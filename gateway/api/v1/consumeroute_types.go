// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type OauthConfig struct {
	// TokenRequest is the type of token request, "body" or "header"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=body;header
	TokenRequest string `json:"tokenRequest,omitempty"`
	// GrantType is the grant type for the external IDP authentication
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=client_credentials;authorization_code;password
	GrantType string `json:"grantType,omitempty"`
	// ClientId is the client ID for the external IDP authentication
	// +kubebuilder:validation:Optional
	ClientId string `json:"clientId,omitempty"`
	// ClientSecret is the client secret for the external IDP authentication
	// +kubebuilder:validation:Optional
	ClientSecret string `json:"clientSecret,omitempty"`
	// Scopes for the external IDP authentication
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`
}

// ConsumeRouteSpec defines the desired state of ConsumeRoute
type ConsumeRouteSpec struct {
	Route        types.ObjectRef `json:"route"`
	ConsumerName string          `json:"consumerName"`
	OauthConfig  OauthConfig     `json:"oauthConfig"`
}

// ConsumeRouteStatus defines the observed state of ConsumeRoute
type ConsumeRouteStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ConsumeRoute is the Schema for the consumeroutes API
type ConsumeRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConsumeRouteSpec   `json:"spec,omitempty"`
	Status ConsumeRouteStatus `json:"status,omitempty"`
}

var _ types.Object = &ConsumeRoute{}

func (c *ConsumeRoute) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

func (c *ConsumeRoute) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&c.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ConsumeRouteList contains a list of ConsumeRoute
type ConsumeRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConsumeRoute `json:"items"`
}

var _ types.ObjectList = &ConsumeRouteList{}

func (c *ConsumeRouteList) GetItems() []types.Object {
	items := make([]types.Object, len(c.Items))
	for i := range c.Items {
		items[i] = &c.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ConsumeRoute{}, &ConsumeRouteList{})
}
