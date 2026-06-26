// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// ZoneServiceConfigSpec defines the file-domain service configuration for one zone.
type ZoneServiceConfigSpec struct {
	// +kubebuilder:validation:Required
	API sftpv1.APIEndpoint `json:"api"`

	// Service is the internal SFTP service endpoint.
	// +kubebuilder:validation:Required
	Service sftpv1.ServiceEndpoint `json:"service"`

	// ServiceExternal is the externally reachable SFTP service endpoint.
	// +kubebuilder:validation:Required
	ServiceExternal sftpv1.ServiceEndpoint `json:"serviceExternal"`
}

// ZoneServiceConfigStatus defines the observed state of ZoneServiceConfig.
type ZoneServiceConfigStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// SFTPZoneServiceConfigRef references the projected SFTP ZoneServiceConfig.
	// +optional
	SFTPZoneServiceConfigRef *types.ObjectRef `json:"sftpZoneServiceConfigRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Service Host",type="string",JSONPath=".spec.service.host"
// +kubebuilder:printcolumn:name="Service Port",type="integer",JSONPath=".spec.service.port"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ZoneServiceConfig is the Schema for the zoneserviceconfigs API.
type ZoneServiceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZoneServiceConfigSpec   `json:"spec,omitempty"`
	Status ZoneServiceConfigStatus `json:"status,omitempty"`
}

var _ types.Object = &ZoneServiceConfig{}

func (r *ZoneServiceConfig) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *ZoneServiceConfig) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ZoneServiceConfigList contains a list of ZoneServiceConfig.
type ZoneServiceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZoneServiceConfig `json:"items"`
}

var _ types.ObjectList = &ZoneServiceConfigList{}

func (r *ZoneServiceConfigList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ZoneServiceConfig{}, &ZoneServiceConfigList{})
}
