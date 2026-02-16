// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

const (
	// SubscriptionAPIVersion is the Kubernetes API version for Subscription resources
	// sent to the configuration backend.
	SubscriptionAPIVersion = "subscriber.horizon.telekom.de/v1"

	// SubscriptionKind is the Kubernetes kind for Subscription resources.
	SubscriptionKind = "Subscription"

	// DefaultDataplaneNamespace is used as a placeholder namespace until
	// EventStoreSpec is extended with a DataplaneNamespace field.
	DefaultDataplaneNamespace = "default"
)

// SubscriptionResource is the full Kubernetes-style resource envelope sent to
// the configuration backend. The configuration API expects a valid Kubernetes resource with
// apiVersion, kind, metadata, and spec fields.
type SubscriptionResource struct {
	// ApiVersion is the Kubernetes API version (e.g. "subscriber.horizon.telekom.de/v1").
	ApiVersion string `json:"apiVersion"`

	// Kind is the Kubernetes resource kind (e.g. "Subscription").
	Kind string `json:"kind"`

	// Metadata contains the resource identity fields.
	Metadata SubscriptionMetadata `json:"metadata"`

	// Spec contains the subscription specification.
	Spec SubscriptionSpec `json:"spec"`
}

// SubscriptionMetadata holds the Kubernetes-style metadata fields needed by the
// configuration backend. Only name and namespace are required.
type SubscriptionMetadata struct {
	// Name is the subscription resource name (the subscription ID).
	Name string `json:"name"`

	// Namespace is the target dataplane namespace.
	Namespace string `json:"namespace"`
}

// SubscriptionSpec wraps the subscription body and environment
type SubscriptionSpec struct {
	// Environment is the realm/environment name.
	// +optional
	Environment string `json:"environment,omitempty"`

	// Subscription contains the actual subscription configuration.
	Subscription SubscriptionPayload `json:"subscription"`
}

// SubscriptionTriggerPayload defines filtering criteria sent to the configuration backend.
type SubscriptionTriggerPayload struct {
	// ResponseFilterMode controls whether the response filter includes or excludes the specified fields.
	// +optional
	ResponseFilterMode string `json:"responseFilterMode,omitempty"`

	// ResponseFilter lists the JSON paths to include or exclude from the event payload.
	// +optional
	ResponseFilter []string `json:"responseFilter,omitempty"`

	// SelectionFilter defines simple key-value equality matches on CloudEvents attributes.
	// +optional
	SelectionFilter map[string]string `json:"selectionFilter,omitempty"`

	// AdvancedSelectionFilter contains an arbitrary JSON filter expression tree.
	// +optional
	AdvancedSelectionFilter map[string]any `json:"advancedSelectionFilter,omitempty"`
}

// SubscriptionPayload contains the subscription fields nested inside
// SubscriptionSpec.Subscription.
type SubscriptionPayload struct {
	// SubscriptionId is the self-assigned subscription identifier.
	// +optional
	SubscriptionId string `json:"subscriptionId,omitempty"`

	// SubscriberId is the unique identifier for the subscriber.
	SubscriberId string `json:"subscriberId"`

	// PublisherId is the unique identifier for the publisher.
	PublisherId string `json:"publisherId"`

	// CreatedAt is the timestamp when the subscription was created.
	// +optional
	CreatedAt string `json:"createdAt,omitempty"`

	// Trigger defines subscriber-side filtering criteria for event delivery.
	// +optional
	Trigger *SubscriptionTriggerPayload `json:"trigger,omitempty"`

	// Type is the event type identifier.
	// +optional
	Type string `json:"type,omitempty"`

	// Callback is the URL where events are delivered for callback-type subscriptions.
	// +optional
	Callback string `json:"callback,omitempty"`

	// PayloadType defines the event payload format (e.g. "data", "dataref").
	// +optional
	PayloadType string `json:"payloadType,omitempty"`

	// DeliveryType defines the delivery mechanism (e.g. "Callback", "ServerSentEvent").
	// +optional
	DeliveryType string `json:"deliveryType,omitempty"`

	// AdditionalPublisherIds allows multiple application IDs to publish to the same event type.
	// +optional
	AdditionalPublisherIds []string `json:"additionalPublisherIds,omitempty"`

	// AppliedScopes lists the scope names that this subscriber is subscribed to.
	// +optional
	AppliedScopes []string `json:"appliedScopes,omitempty"`

	// EventRetentionTime defines how long events are retained for this subscriber.
	// +optional
	EventRetentionTime string `json:"eventRetentionTime,omitempty"`

	// CircuitBreakerOptOut disables the circuit breaker for this subscription.
	// +optional
	CircuitBreakerOptOut *bool `json:"circuitBreakerOptOut,omitempty"`

	// RetryableStatusCodes defines HTTP status codes that should trigger a retry.
	// +optional
	RetryableStatusCodes []int `json:"retryableStatusCodes,omitempty"`

	// RedeliveriesPerSecond limits the rate of event redeliveries.
	// +optional
	RedeliveriesPerSecond *int `json:"redeliveriesPerSecond,omitempty"`

	// PublisherTrigger defines publisher-side filtering criteria applied to this subscriber.
	// +optional
	PublisherTrigger *SubscriptionTriggerPayload `json:"publisherTrigger,omitempty"`

	// EnforceGetHttpRequestMethodForHealthCheck forces GET for health check probes instead of HEAD.
	// +optional
	EnforceGetHttpRequestMethodForHealthCheck *bool `json:"enforceGetHttpRequestMethodForHealthCheck,omitempty"`
}
