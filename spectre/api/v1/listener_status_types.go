// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

// AppliedListenerPlacementStatus records the durable placement state that was
// last successfully reconciled. Controllers compare the current fingerprint
// against the desired one to decide whether a re-placement is needed.
type AppliedListenerPlacementStatus struct {
	// +optional
	Fingerprint string `json:"fingerprint,omitempty"`
	// +optional
	CaptureRoute *ctypes.ObjectRef `json:"captureRoute,omitempty"`
	// +optional
	CaptureZone *ctypes.ObjectRef `json:"captureZone,omitempty"`
	// +optional
	CaptureEventStore *ctypes.ObjectRef `json:"captureEventStore,omitempty"`
	// +optional
	CallbackOriginZone *ctypes.ObjectRef `json:"callbackOriginZone,omitempty"`
	// +optional
	DeliveryZone *ctypes.ObjectRef `json:"deliveryZone,omitempty"`
	// +optional
	DeliveryEventStore *ctypes.ObjectRef `json:"deliveryEventStore,omitempty"`
	// +optional
	Publisher *ctypes.ObjectRef `json:"publisher,omitempty"`
	// +optional
	CallbackBaseURL string `json:"callbackBaseURL,omitempty"`
}

// ListenerDrainStatus tracks the progress of tearing down an old placement.
// The controller walks through the phases in order: stop new traffic, drain
// subscribers, clean up the publisher, then mark complete.
type ListenerDrainStatus struct {
	// +kubebuilder:validation:Enum=Stopping;DrainingSubscribers;CleaningPublisher;Complete
	Phase string `json:"phase"`
	// +optional
	Reason string `json:"reason,omitempty"`
	// +optional
	OldFingerprint string `json:"oldFingerprint,omitempty"`
	// OldRouteListeners records every old-generation RouteListener that must be
	// gone before Subscribers drain, sorted by namespace, name, UID.
	// +optional
	OldRouteListeners []ctypes.ObjectRef `json:"oldRouteListeners,omitempty"`
	// +optional
	OldSubscribers []ctypes.ObjectRef `json:"oldSubscribers,omitempty"`
	// +optional
	SourcePublisher *ctypes.ObjectRef `json:"sourcePublisher,omitempty"`
	// +optional
	SourceEventStore *ctypes.ObjectRef `json:"sourceEventStore,omitempty"`
	// +optional
	SubscriptionIDs []string `json:"subscriptionIDs,omitempty"`
}
