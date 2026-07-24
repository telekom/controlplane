// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
)

// ZoneServiceConfigSpec defines the file-domain service configuration for one zone.
// A ZoneServiceConfig must use the same name and namespace as its admin Zone.
type ZoneServiceConfigSpec struct {
	// +kubebuilder:validation:Required
	API adminv1.ManagedRouteConfig `json:"api"`

	// Service is the internal SFTP service endpoint.
	// +kubebuilder:validation:Optional
	Service *ServiceEndpoint `json:"service"`

	// ServiceExternal is the externally reachable SFTP service endpoint.
	// +kubebuilder:validation:Optional
	ServiceExternal *ServiceEndpoint `json:"serviceExternal"`
}

// ServiceEndpoint identifies an SFTP service endpoint.
type ServiceEndpoint struct {
	// Host is the hostname or IP address of the service.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Port is the TCP port of the service.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// ZoneServiceConfigStatus defines the observed state of ZoneServiceConfig.
type ZoneServiceConfigStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// SFTPServiceConfigRef references the projected SFTPServiceConfig.
	// +optional
	SFTPServiceConfigRef *types.ObjectRef `json:"sftpServiceConfigRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="API URL",type="string",JSONPath=".spec.api.url"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ZoneServiceConfig is the Schema for the zoneserviceconfigs API.
// It must use the same name and namespace as the admin Zone it configures.
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
