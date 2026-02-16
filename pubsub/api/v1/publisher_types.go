// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

// PublisherSpec defines the desired state of Publisher.
// Publisher represents a registered event publisher in the configuration backend.
// It is created by the EventExposure handler in the event domain.
type PublisherSpec struct {
	// EventStore references the EventStore CR that provides configuration connection details.
	EventStore ctypes.ObjectRef `json:"eventStore"`

	// EventType is the dot-separated event type identifier (e.g. "de.telekom.eni.quickstart.v1").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EventType string `json:"eventType"`

	// PublisherId is the unique identifier for this publisher in the configuration backend.
	// Typically derived from the providing application's identifier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	PublisherId string `json:"publisherId"`

	// AdditionalPublisherIds allows multiple application IDs to publish to the same event type.
	// +optional
	AdditionalPublisherIds []string `json:"additionalPublisherIds,omitempty"`
}

// PublisherStatus defines the observed state of Publisher.
type PublisherStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Publisher is the Schema for the publishers API.
// It represents an event publisher registration in the configuration backend.
// Publisher resources are created and managed by the EventExposure handler in the event domain.
type Publisher struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PublisherSpec   `json:"spec,omitempty"`
	Status PublisherStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &Publisher{}

func (r *Publisher) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Publisher) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// PublisherList contains a list of Publisher
type PublisherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Publisher `json:"items"`
}

var _ ctypes.ObjectList = &PublisherList{}

func (r *PublisherList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Publisher{}, &PublisherList{})
}
