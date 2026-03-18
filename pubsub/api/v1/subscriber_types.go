// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

// SubscriberSpec defines the desired state of Subscriber.
// Subscriber represents an event subscription registration in the configuration backend.
// It is created by the EventSubscription handler in the event domain.
type SubscriberSpec struct {
	// Publisher references the Publisher CR that this subscriber subscribes to.
	// The Subscriber controller resolves the Publisher at runtime to obtain
	// EventType, PublisherId, AdditionalPublisherIds, and EventStore details.
	Publisher ctypes.ObjectRef `json:"publisher"`

	// SubscriberId is the unique identifier for this subscriber in the configuration backend.
	// Typically derived from the consuming application's identifier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SubscriberId string `json:"subscriberId"`

	// Delivery configures how events are delivered to the subscriber.
	Delivery SubscriptionDelivery `json:"delivery"`

	// Trigger defines subscriber-side filtering criteria for event delivery.
	// +optional
	Trigger *Trigger `json:"trigger,omitempty"`

	// PublisherTrigger defines publisher-side filtering criteria for event delivery.
	// +optional
	PublisherTrigger *Trigger `json:"publisherTrigger,omitempty"`

	// AppliedScopes lists the scope names that are applied to this subscription.
	// +optional
	AppliedScopes []string `json:"appliedScopes,omitempty"`
}

// SubscriberStatus defines the observed state of Subscriber.
type SubscriberStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// SubscriptionId is the self-assigned subscription identifier.
	// Populated after the subscription is successfully registered with the configuration backend.
	// +optional
	SubscriptionId string `json:"subscriptionId,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Subscriber is the Schema for the subscribers API.
// It represents an event subscription registration in the configuration backend.
// Subscriber resources are created and managed by the EventSubscription handler in the event domain.
type Subscriber struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubscriberSpec   `json:"spec,omitempty"`
	Status SubscriberStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &Subscriber{}

func (r *Subscriber) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Subscriber) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// SubscriberList contains a list of Subscriber
type SubscriberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subscriber `json:"items"`
}

var _ ctypes.ObjectList = &SubscriberList{}

func (r *SubscriberList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Subscriber{}, &SubscriberList{})
}
