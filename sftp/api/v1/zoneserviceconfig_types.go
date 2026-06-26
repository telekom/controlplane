// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// ZoneServiceConfigSpec defines the desired state of ZoneServiceConfig
type ZoneServiceConfigSpec struct {
	// API contains authentication configuration for API service access.
	// +kubebuilder:validation:Required
	API APIEndpoint `json:"api"`
}

type APIEndpoint struct {
	// Endpoint is the SFTP Tardis API base URL.
	Endpoint string `json:"endpoint"`
	// Issuer is the OAuth2 token endpoint used for client credentials authentication.
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientID,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// ZoneServiceConfigStatus defines the observed state of ZoneServiceConfig
type ZoneServiceConfigStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	Generation int64 `json:"generation,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="API Endpoint",type="string",JSONPath=".spec.api.endpoint"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ZoneServiceConfig is the Schema for the zoneserviceconfigs API
type ZoneServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZoneServiceConfigSpec   `json:"spec,omitempty"`
	Status ZoneServiceConfigStatus `json:"status,omitempty"`
}

var _ types.Object = &ZoneServiceConfig{}

func (z *ZoneServiceConfig) GetConditions() []metav1.Condition {
	return z.Status.Conditions
}

func (z *ZoneServiceConfig) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&z.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ZoneServiceConfigList contains a list of ZoneServiceConfig
type ZoneServiceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZoneServiceConfig `json:"items"`
}

var _ types.ObjectList = &ZoneServiceConfigList{}

func (zl *ZoneServiceConfigList) GetItems() []types.Object {
	items := make([]types.Object, len(zl.Items))
	for i := range zl.Items {
		items[i] = &zl.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ZoneServiceConfig{}, &ZoneServiceConfigList{})
}
