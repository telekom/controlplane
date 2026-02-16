// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// EventDeliveryType defines how events are delivered to subscribers.
// +kubebuilder:validation:Enum=Callback;ServerSentEvent
type EventDeliveryType string

const (
	EventDeliveryTypeCallback        EventDeliveryType = "Callback"
	EventDeliveryTypeServerSentEvent EventDeliveryType = "ServerSentEvent"
)

// EventPayloadType defines the event payload format.
// +kubebuilder:validation:Enum=Data;DataRef
type EventPayloadType string

const (
	EventPayloadTypeData    EventPayloadType = "Data"
	EventPayloadTypeDataRef EventPayloadType = "DataRef"
)

// EventDelivery configures how events are delivered to the subscriber.
// +kubebuilder:validation:XValidation:rule="self.type == 'Callback' ? self.callback != \"\" : !has(self.callback)",message="callback is required for deliveryType 'Callback' and must not be set for 'ServerSentEvent'"
type EventDelivery struct {
	// Type defines the delivery mechanism.
	// +kubebuilder:default=Callback
	Type EventDeliveryType `json:"type"`

	// Payload defines the event payload format.
	// +kubebuilder:default=Data
	Payload EventPayloadType `json:"payload"`

	// Callback is the URL where events are delivered.
	// Required when type is "callback", must not be set for "ServerSentEvent".
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:Optional
	Callback string `json:"callback,omitempty"`

	// EventRetentionTime defines how long events are retained for this subscriber.
	// +kubebuilder:validation:Format=duration
	// +kubebuilder:validation:Optional
	EventRetentionTime string `json:"eventRetentionTime,omitempty"`

	// CircuitBreakerOptOut disables the circuit breaker for this subscription.
	// +kubebuilder:validation:Optional
	CircuitBreakerOptOut bool `json:"circuitBreakerOptOut,omitempty"`

	// RetryableStatusCodes defines HTTP status codes that should trigger a retry.
	// +kubebuilder:validation:Optional
	RetryableStatusCodes []int `json:"retryableStatusCodes,omitempty"`

	// RedeliveriesPerSecond limits the rate of event redeliveries.
	// +kubebuilder:validation:Optional
	RedeliveriesPerSecond *int `json:"redeliveriesPerSecond,omitempty"`

	// EnforceGetHttpRequestMethodForHealthCheck forces GET for health check probes instead of HEAD.
	// +kubebuilder:validation:Optional
	EnforceGetHttpRequestMethodForHealthCheck bool `json:"enforceGetHttpRequestMethodForHealthCheck,omitempty"`
}

// EventResponseFilterMode controls whether the response filter includes or excludes the specified fields.
// +kubebuilder:validation:Enum=Include;Exclude
type EventResponseFilterMode string

const (
	EventResponseFilterModeInclude EventResponseFilterMode = "Include"
	EventResponseFilterModeExclude EventResponseFilterMode = "Exclude"
)

// EventResponseFilter controls which fields are included or excluded from the event payload.
type EventResponseFilter struct {
	// Paths lists the JSON paths to include or exclude from the event payload.
	// +kubebuilder:validation:Optional
	Paths []string `json:"paths,omitempty"`

	// Mode controls whether the listed paths are included or excluded.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=Include
	Mode EventResponseFilterMode `json:"mode,omitempty"`
}

// EventSelectionFilter defines criteria for selecting which events are delivered.
type EventSelectionFilter struct {
	// Attributes defines simple key-value equality matches on CloudEvents attributes.
	// All entries are AND-ed together.
	// +kubebuilder:validation:Optional
	Attributes map[string]string `json:"attributes,omitempty"`

	// Expression contains an arbitrary JSON filter expression tree
	// using logical operators (and, or) and comparisons (eq, ge, gt, le, lt, ne)
	// that is passed through to the configuration backend without structural validation.
	// +kubebuilder:validation:Optional
	Expression *apiextensionsv1.JSON `json:"expression,omitempty"`
}

// EventTrigger defines filtering criteria for event delivery.
type EventTrigger struct {
	// ResponseFilter controls payload shaping (which fields to return).
	// +kubebuilder:validation:Optional
	ResponseFilter *EventResponseFilter `json:"responseFilter,omitempty"`

	// SelectionFilter controls event matching (which events to deliver).
	// +kubebuilder:validation:Optional
	SelectionFilter *EventSelectionFilter `json:"selectionFilter,omitempty"`
}

// EventScope defines a named scope with optional trigger-based filtering for event exposure.
// Scopes allow publishers to partition their events and apply publisher-side filters.
type EventScope struct {
	// Name is the unique identifier for this scope.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Trigger defines publisher-side filtering criteria for this scope.
	// +kubebuilder:validation:Optional
	Trigger *EventTrigger `json:"trigger,omitempty"`
}
