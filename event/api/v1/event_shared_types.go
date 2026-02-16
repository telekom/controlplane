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

// EventTrigger defines filtering criteria for event delivery.
type EventTrigger struct {
	// ResponseFilter controls payload shaping (which fields to return).
	// +optional
	ResponseFilter *ResponseFilter `json:"responseFilter,omitempty"`

	// SelectionFilter controls event matching (which events to deliver).
	// +optional
	SelectionFilter *SelectionFilter `json:"selectionFilter,omitempty"`
}

// EventScope defines a named scope with optional trigger-based filtering for event exposure.
// Scopes allow publishers to partition their events and apply publisher-side filters.
type EventScope struct {
	// Name is the unique identifier for this scope.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Trigger defines publisher-side filtering criteria for this scope.
	// +optional
	Trigger *EventTrigger `json:"trigger,omitempty"`
}
