// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// SFTPServiceConfigSpec defines the desired state of SFTPServiceConfig
type SFTPServiceConfigSpec struct {
	// API contains authentication configuration for API service access.
	// +kubebuilder:validation:Required
	API APIEndpoint `json:"api"`
}

type APIEndpoint struct {
	// Endpoint is the SFTP Tardis API base URL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=uri
	Endpoint string `json:"endpoint"`

	// Issuer is the OAuth2 token endpoint used for client credentials authentication.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=uri
	Issuer string `json:"issuer"`

	// ClientID is the OAuth2 client ID used for client credentials authentication.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientID"`

	// ClientSecret is the OAuth2 client secret used for client credentials authentication.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientSecret string `json:"clientSecret"`
}

// SFTPServiceConfigStatus defines the observed state of SFTPServiceConfig
type SFTPServiceConfigStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="API Endpoint",type="string",JSONPath=".spec.api.endpoint"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SFTPServiceConfig is the Schema for the sftpserviceconfigs API
type SFTPServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SFTPServiceConfigSpec   `json:"spec,omitempty"`
	Status SFTPServiceConfigStatus `json:"status,omitempty"`
}

var _ types.Object = &SFTPServiceConfig{}

func (z *SFTPServiceConfig) GetConditions() []metav1.Condition {
	return z.Status.Conditions
}

func (z *SFTPServiceConfig) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&z.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// SFTPServiceConfigList contains a list of SFTPServiceConfig
type SFTPServiceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SFTPServiceConfig `json:"items"`
}

var _ types.ObjectList = &SFTPServiceConfigList{}

func (zl *SFTPServiceConfigList) GetItems() []types.Object {
	items := make([]types.Object, len(zl.Items))
	for i := range zl.Items {
		items[i] = &zl.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&SFTPServiceConfig{}, &SFTPServiceConfigList{})
}
