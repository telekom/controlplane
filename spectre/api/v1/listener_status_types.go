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
	// +optional
	OldRouteListener *ctypes.ObjectRef `json:"oldRouteListener,omitempty"`
	// +optional
	OldSubscribers []ctypes.ObjectRef `json:"oldSubscribers,omitempty"`
	// +optional
	SourcePublisher *ctypes.ObjectRef `json:"sourcePublisher,omitempty"`
	// +optional
	SourceEventStore *ctypes.ObjectRef `json:"sourceEventStore,omitempty"`
	// +optional
	SubscriptionIDs []string `json:"subscriptionIDs,omitempty"`
}

// AuthorizationMigrationStatus tracks the transition from a legacy
// (provider-only) approval model to the scoped dual-gate model. Once the
// migration completes, the controller sets AuthorizationPolicyVersion="v2"
// on the parent ListenerStatus and clears this struct.
type AuthorizationMigrationStatus struct {
	// +optional
	TargetPolicyVersion string `json:"targetPolicyVersion,omitempty"`
	// +kubebuilder:validation:Enum=Recorded;Blocked;AwaitingScoped;Draining;RetiringRequests;RetiringApproval
	Phase string `json:"phase"`
	// +optional
	LegacyApproval *ctypes.ObjectRef `json:"legacyApproval,omitempty"`
	// +optional
	LegacyRequests []ctypes.ObjectRef `json:"legacyRequests,omitempty"`
	// +optional
	RetirementCheckpoint *MigrationRetirementCheckpoint `json:"retirementCheckpoint,omitempty"`
	// DrainStarted is set to true when the migration initiates a drain.
	// It distinguishes "drain not yet started" (false) from "drain was
	// consumed by the outer handler" (true but Draining is nil).
	// +optional
	DrainStarted bool `json:"drainStarted,omitempty"`
}

// MigrationRetirementCheckpoint records which legacy approval resources have
// been retired so the controller can resume after a restart.
type MigrationRetirementCheckpoint struct {
	// +optional
	RequestsRetired bool `json:"requestsRetired,omitempty"`
	// +optional
	ApprovalRetired bool `json:"approvalRetired,omitempty"`
	// +optional
	LastRetiredUID string `json:"lastRetiredUID,omitempty"`
	// +optional
	LastRetiredResourceVersion string `json:"lastRetiredResourceVersion,omitempty"`
	// +optional
	PendingDeletions []PendingDeletion `json:"pendingDeletions,omitempty"`
}

// PendingDeletion tracks per-resource deletion state so the controller can
// survive a crash between issuing a delete and acknowledging the result.
type PendingDeletion struct {
	// +optional
	Kind string `json:"kind,omitempty"`
	// +optional
	Name string `json:"name,omitempty"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// +optional
	UID string `json:"uid,omitempty"`
	// +optional
	ResourceVersion string `json:"resourceVersion,omitempty"`
	// +kubebuilder:validation:Enum=DeletePrepared;DeleteObserved
	// +optional
	Phase string `json:"phase,omitempty"`
}
