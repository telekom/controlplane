// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

// EventStoreSpec defines the desired state of EventStore.
// EventStore holds resolved operational values for connecting to the configuration backend,
// including OAuth2 credentials. It is created by the EventConfig handler.
type EventStoreSpec struct {
	// Url is the base URL of the configuration backend API.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=uri
	Url string `json:"url"`

	// TokenUrl is the OAuth2 token endpoint for authenticating with the configuration backend.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=uri
	TokenUrl string `json:"tokenUrl"`

	// ClientId is the OAuth2 client ID for authenticating with the configuration backend.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientId string `json:"clientId"`

	// ClientSecret is the OAuth2 client secret for authenticating with the configuration backend.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientSecret string `json:"clientSecret"`
}

// EventStoreStatus defines the observed state of EventStore.
type EventStoreStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EventStore is the Schema for the eventstores API.
// It stores the resolved connection and authentication details for the configuration backend.
// EventStore resources are created and managed by the EventConfig handler in the event domain.
type EventStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventStoreSpec   `json:"spec,omitempty"`
	Status EventStoreStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &EventStore{}

func (r *EventStore) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *EventStore) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// EventStoreList contains a list of EventStore
type EventStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventStore `json:"items"`
}

var _ ctypes.ObjectList = &EventStoreList{}

func (r *EventStoreList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&EventStore{}, &EventStoreList{})
}
