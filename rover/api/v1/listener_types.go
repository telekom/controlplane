// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// RoverListener defines an API traffic listener that observes requests/responses
// between a consumer and provider on a specific API path.
// Consumer and provider accept either the full application ID "<group>--<team>--<name>"
// or the bare application name, which must be unique across all teams. Any value with
// two or more "--" is read as a full ID, so a bare name must not contain "--"; use the
// full ID instead.
// The Listener name is derived from the consumer and provider values as written, so
// changing either value, including switching the same application between bare name
// and full ID, recreates the Listener and requires a new approval.
type RoverListener struct {
	// Consumer is the consuming application, as full ID (e.g. "eni--team--app") or bare name (e.g. "app").
	// A bare name must not contain "--"; use the full ID instead.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Consumer string `json:"consumer"`

	// Provider is the providing application, as full ID (e.g. "eni--other--provider") or bare name (e.g. "provider").
	// A bare name must not contain "--"; use the full ID instead.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// ApiBasePath is the base path of the API to listen on
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^/.*$`
	ApiBasePath string `json:"apiBasePath,omitempty"`

	// EventType is the event type to listen on (alternative to ApiBasePath)
	// +kubebuilder:validation:Optional
	EventType string `json:"eventType,omitempty"`

	// RequestFilter defines filtering for request traffic
	// +kubebuilder:validation:Optional
	RequestFilter *ListenerFilter `json:"requestFilter,omitempty"`

	// ResponseFilter defines filtering for response traffic
	// +kubebuilder:validation:Optional
	ResponseFilter *ListenerFilter `json:"responseFilter,omitempty"`

	// EventFilter defines filtering for event traffic
	// +kubebuilder:validation:Optional
	EventFilter *ListenerFilter `json:"eventFilter,omitempty"`
}

// ListenerFilter defines trigger conditions and payload field selection for listener filtering.
type ListenerFilter struct {
	// Trigger defines key-value conditions that must match for the filter to activate
	// +kubebuilder:validation:Optional
	Trigger map[string]string `json:"trigger,omitempty"`

	// Payload defines the list of fields to include in the listener output
	// +kubebuilder:validation:Optional
	Payload []string `json:"payload,omitempty"`
}

// ListenerSubscription configures how listener events are delivered to the subscriber.
// +kubebuilder:validation:XValidation:rule="self.deliveryType == 'callback' ? (has(self.callback) && self.callback != '') : !has(self.callback)",message="callback is required for deliveryType 'callback' and must not be set otherwise; set deliveryType: callback to deliver to a callback URL"
type ListenerSubscription struct {
	// DeliveryType defines the delivery mechanism for listener events
	// +kubebuilder:validation:Enum=callback;server_sent_event
	// +kubebuilder:default=server_sent_event
	DeliveryType string `json:"deliveryType,omitempty"`

	// Callback is the URL where listener events are delivered when deliveryType is "callback"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=uri
	Callback string `json:"callback,omitempty"`
}
