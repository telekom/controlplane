// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Delivery configures how events are delivered to the subscriber.
// +kubebuilder:validation:XValidation:rule="self.type == 'Callback' ? self.callback != \"\" : !has(self.callback)",message="callback is required for deliveryType 'Callback' and must not be set for 'ServerSentEvent'"
type Delivery struct {
	// Type defines the delivery mechanism.
	// +kubebuilder:default=Callback
	Type DeliveryType `json:"type"`

	// Payload defines the event payload format.
	// +kubebuilder:default=Data
	Payload PayloadType `json:"payload"`

	// Callback is the URL where events are delivered.
	// Required when type is "callback", must not be set for "ServerSentEvent".
	// +kubebuilder:validation:Format=uri
	// +optional
	Callback string `json:"callback,omitempty"`

	// EventRetentionTime defines how long events are retained for this subscriber.
	// +optional
	EventRetentionTime string `json:"eventRetentionTime,omitempty"`

	// CircuitBreakerOptOut disables the circuit breaker for this subscription.
	// +optional
	CircuitBreakerOptOut bool `json:"circuitBreakerOptOut,omitempty"`

	// RetryableStatusCodes defines HTTP status codes that should trigger a retry.
	// +optional
	RetryableStatusCodes []int `json:"retryableStatusCodes,omitempty"`

	// RedeliveriesPerSecond limits the rate of event redeliveries.
	// +optional
	RedeliveriesPerSecond *int `json:"redeliveriesPerSecond,omitempty"`

	// EnforceGetHttpRequestMethodForHealthCheck forces GET for health check probes instead of HEAD.
	// +optional
	EnforceGetHttpRequestMethodForHealthCheck bool `json:"enforceGetHttpRequestMethodForHealthCheck,omitempty"`
}

// EventSubscriptionSpec defines the desired state of EventSubscription.
type EventSubscriptionSpec struct {
	// EventType is the dot-separated event type identifier (e.g. "de.telekom.eni.quickstart.v1").
	// References the EventType CR via MakeEventTypeName() conversion.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EventType string `json:"eventType"`

	// Zone references the Zone CR where this subscription is placed.
	Zone ctypes.ObjectRef `json:"zone"`

	// Requestor identifies the consuming application.
	Requestor ctypes.TypedObjectRef `json:"requestor"`

	// Delivery configures how events are delivered to the subscriber.
	Delivery Delivery `json:"delivery"`

	// Trigger defines subscriber-side filtering criteria for event delivery.
	// +optional
	Trigger *EventTrigger `json:"trigger,omitempty"`

	// Scopes selects which publisher-defined scopes to subscribe to.
	// Must match scope names defined on the corresponding EventExposure.
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// TODO: Add Security field — currently derived from Zone/Gateway config in the handler
}

// EventSubscriptionStatus defines the observed state of EventSubscription.
type EventSubscriptionStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Subscriber references the Subscriber CR in the pubsub domain.
	// +optional
	Subscriber *ctypes.ObjectRef `json:"subscriber,omitempty"`

	// Approval references the Approval CR managing subscription approval.
	// +optional
	Approval *ctypes.ObjectRef `json:"approval,omitempty"`

	// ApprovalRequest references the ApprovalRequest CR for this subscription.
	// +optional
	ApprovalRequest *ctypes.ObjectRef `json:"approvalRequest,omitempty"`

	// URL is the SSE endpoint URL for this subscription, set when Delivery.Type is ServerSentEvent.
	// +kubebuilder:validation:Format=uri
	// +optional
	URL string `json:"url,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="EventType",type="string",JSONPath=".spec.eventType",description="The event type identifier"
// +kubebuilder:printcolumn:name="CreatedAt",type="date",JSONPath=".metadata.creationTimestamp",description="Creation timestamp"

// EventSubscription is the Schema for the eventsubscriptions API.
// It represents a declaration that an application subscribes to events of a specific type,
// configuring delivery, filtering, and scope selection.
type EventSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventSubscriptionSpec   `json:"spec,omitempty"`
	Status EventSubscriptionStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &EventSubscription{}

func (r *EventSubscription) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *EventSubscription) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// EventSubscriptionList contains a list of EventSubscription
type EventSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventSubscription `json:"items"`
}

var _ ctypes.ObjectList = &EventSubscriptionList{}

func (r *EventSubscriptionList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&EventSubscription{}, &EventSubscriptionList{})
}
