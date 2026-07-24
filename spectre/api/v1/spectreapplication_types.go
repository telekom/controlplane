// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpectreApplicationSpec defines the desired state of SpectreApplication.
// +kubebuilder:validation:XValidation:rule="self.deliveryType == 'callback' ? has(self.callback) : !has(self.callback)",message="callback is required when deliveryType is 'callback' and must not be set otherwise"
type SpectreApplicationSpec struct {
	Application ctypes.TypedObjectRef `json:"application"`
	// +kubebuilder:validation:Enum=callback;server_sent_event
	// +kubebuilder:default=server_sent_event
	// +optional
	DeliveryType string `json:"deliveryType,omitempty"`
	// +optional
	Callback string `json:"callback,omitempty"`
}

// SpectreApplicationStatus defines the observed state of SpectreApplication.
type SpectreApplicationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// +optional
	Id string `json:"id,omitempty"`
	// +optional
	Publisher *ctypes.ObjectRef `json:"publisher,omitempty"`
	// +optional
	Subscriber *ctypes.ObjectRef `json:"subscriber,omitempty"`
	// +optional
	ListenerRoute *ctypes.ObjectRef `json:"listenerRoute,omitempty"`
	// +optional
	ProxyRoute *ctypes.ObjectRef `json:"proxyRoute,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SpectreApplication is the Schema for the spectreapplications API.
type SpectreApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SpectreApplicationSpec `json:"spec"`
	// +optional
	Status SpectreApplicationStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &SpectreApplication{}

func (a *SpectreApplication) GetConditions() []metav1.Condition {
	return a.Status.Conditions
}

func (a *SpectreApplication) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&a.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// SpectreApplicationList contains a list of SpectreApplication.
type SpectreApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SpectreApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SpectreApplication{}, &SpectreApplicationList{})
}
