// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Visibility defines who can see and subscribe to an exposed event.
// +kubebuilder:validation:Enum=World;Zone;Enterprise
type Visibility string

const (
	VisibilityWorld      Visibility = "World"
	VisibilityZone       Visibility = "Zone"
	VisibilityEnterprise Visibility = "Enterprise"
)

func (v Visibility) String() string {
	return string(v)
}

// ApprovalStrategy defines the approval mode for subscriptions.
// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
type ApprovalStrategy string

const (
	ApprovalStrategyAuto     ApprovalStrategy = "Auto"
	ApprovalStrategySimple   ApprovalStrategy = "Simple"
	ApprovalStrategyFourEyes ApprovalStrategy = "FourEyes"
)

// Approval configures how subscriptions to this event are approved.
type Approval struct {
	// Strategy defines the approval mode.
	// +kubebuilder:default=Auto
	Strategy ApprovalStrategy `json:"strategy"`

	// TrustedTeams identifies teams that are trusted for approving subscriptions.
	// By default your own team is trusted.
	// +optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	TrustedTeams []string `json:"trustedTeams,omitempty"`
}

// EventExposureSpec defines the desired state of EventExposure.
type EventExposureSpec struct {
	// EventType is the dot-separated event type identifier (e.g. "de.telekom.eni.quickstart.v1").
	// References the EventType CR via MakeEventTypeName() conversion.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EventType string `json:"eventType"`

	// Visibility defines who can see and subscribe to this event.
	// +kubebuilder:default=Enterprise
	Visibility Visibility `json:"visibility"`

	// Approval configures how subscriptions to this event are approved.
	Approval Approval `json:"approval"`

	// Zone references the Zone CR where this event is exposed.
	Zone ctypes.ObjectRef `json:"zone"`

	// Provider identifies the providing application.
	Provider ctypes.TypedObjectRef `json:"provider"`

	// Scopes defines named scopes with optional publisher-side trigger filtering.
	// +optional
	Scopes []EventScope `json:"scopes,omitempty"`

	// AdditionalPublisherIds allows multiple application IDs to publish to the same event type.
	// Todo: rethink this approach and consider a decoupling
	// +optional
	AdditionalPublisherIds []string `json:"additionalPublisherIds,omitempty"`

	// TODO: Add Security field — currently derived from Zone/Gateway config in the handler
}

// EventExposureStatus defines the observed state of EventExposure.
type EventExposureStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this EventExposure is the active one for its event type.
	Active bool `json:"active"`

	// Route references the primary gateway Route CR created for this exposure.
	// +optional
	Route *ctypes.ObjectRef `json:"route,omitempty"`

	// ProxyRoutes references proxy gateway Route CRs for cross-zone SSE delivery.
	// +optional
	ProxyRoutes []ctypes.ObjectRef `json:"proxyRoutes,omitempty"`

	SseURLs map[string]string `json:"sseUrls,omitempty"`

	// Publisher references the Publisher CR in the pubsub domain.
	// +optional
	Publisher *ctypes.ObjectRef `json:"publisher,omitempty"`

	// CallbackURL is the URL of callback gateway in the provider zone.
	// +optional
	CallbackURL string `json:"callbackURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="EventType",type="string",JSONPath=".spec.eventType",description="The event type identifier"
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Whether this exposure is active"
// +kubebuilder:printcolumn:name="CreatedAt",type="date",JSONPath=".metadata.creationTimestamp",description="Creation timestamp"

// EventExposure is the Schema for the eventexposures API.
// It represents a declaration that an application publishes events of a specific type,
// making them available for subscription by other applications.
type EventExposure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventExposureSpec   `json:"spec,omitempty"`
	Status EventExposureStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &EventExposure{}

func (r *EventExposure) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *EventExposure) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

func (r *EventExposure) FindSseUrlForZone(zoneName string) (string, bool) {
	url, found := r.Status.SseURLs[zoneName]
	return url, found
}

// +kubebuilder:object:root=true

// EventExposureList contains a list of EventExposure
type EventExposureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventExposure `json:"items"`
}

var _ ctypes.ObjectList = &EventExposureList{}

func (r *EventExposureList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&EventExposure{}, &EventExposureList{})
}
