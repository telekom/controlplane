// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// DeliveryType defines how events are delivered to subscribers.
// +kubebuilder:validation:Enum=Callback;ServerSentEvent
type DeliveryType string

const (
	DeliveryTypeCallback        DeliveryType = "Callback"
	DeliveryTypeServerSentEvent DeliveryType = "ServerSentEvent"
)

func (d DeliveryType) String() string {
	return string(d)
}

// PayloadType defines the event payload format.
// +kubebuilder:validation:Enum=Data;DataRef
type PayloadType string

const (
	PayloadTypeData    PayloadType = "Data"
	PayloadTypeDataRef PayloadType = "DataRef"
)

func (p PayloadType) String() string {
	return string(p)
}

// ResponseFilterMode controls whether the response filter includes or excludes the specified fields.
// +kubebuilder:validation:Enum=Include;Exclude
type ResponseFilterMode string

const (
	ResponseFilterModeInclude ResponseFilterMode = "Include"
	ResponseFilterModeExclude ResponseFilterMode = "Exclude"
)

func (r ResponseFilterMode) String() string {
	return string(r)
}

// ResponseFilter controls which fields are included or excluded from the event payload.
type ResponseFilter struct {
	// Paths lists the JSON paths to include or exclude from the event payload.
	// +optional
	Paths []string `json:"paths,omitempty"`

	// Mode controls whether the listed paths are included or excluded.
	// +optional
	// +kubebuilder:default=Include
	Mode ResponseFilterMode `json:"mode,omitempty"`
}

// SelectionFilter defines criteria for selecting which events are delivered.
type SelectionFilter struct {
	// Attributes defines simple key-value equality matches on CloudEvents attributes.
	// All entries are AND-ed together.
	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`

	// Expression contains an arbitrary JSON filter expression tree
	// using logical operators (and, or) and comparisons (eq, ge, gt, le, lt, ne)
	// that is passed through to the configuration backend without structural validation.
	// +optional
	Expression *apiextensionsv1.JSON `json:"expression,omitempty"`
}

// Trigger defines filtering criteria for event delivery in the pubsub domain.
type Trigger struct {
	// ResponseFilter controls payload shaping (which fields to return).
	// +optional
	ResponseFilter *ResponseFilter `json:"responseFilter,omitempty"`

	// SelectionFilter controls event matching (which events to deliver).
	// +optional
	SelectionFilter *SelectionFilter `json:"selectionFilter,omitempty"`
}

// SubscriptionDelivery configures how events are delivered to the subscriber in the pubsub domain.
// +kubebuilder:validation:XValidation:rule="self.type == 'Callback' ? self.callback != \"\" : !has(self.callback)",message="callback is required for deliveryType 'Callback' and must not be set for 'ServerSentEvent'"
type SubscriptionDelivery struct {
	// Type defines the delivery mechanism.
	// +kubebuilder:default=Callback
	Type DeliveryType `json:"type"`

	// Payload defines the event payload format.
	// +kubebuilder:default=Data
	Payload PayloadType `json:"payload"`

	// Callback is the URL where events are delivered.
	// Required when type is "callback", must not be set for "ServerSentEvent".
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
