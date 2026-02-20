// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var EventTypeLabelKey = config.BuildLabelKey("eventtype")

// MakeEventTypeName generates a Kubernetes resource name from a dot-separated event type identifier.
// It replaces dots with hyphens and lowercases the result (e.g. "de.telekom.eni.quickstart.v1" -> "de-telekom-eni-quickstart-v1").
func MakeEventTypeName(eventType string) string {
	return strings.ToLower(strings.ReplaceAll(eventType, ".", "-"))
}

// EventTypeSpec defines the desired state of EventType.
// +kubebuilder:validation:XValidation:rule="self.type.endsWith('.v' + self.version.split('.')[0])",message="major version in \"version\" must match the version suffix (e.g. \"vN\") in \"type\""
type EventTypeSpec struct {
	// Type is the dot-separated event type identifier (e.g. "de.telekom.eni.quickstart.v1").
	// The last segment must be a version prefix matching the major version.
	// Used to generate the resource name via dots-to-hyphens conversion.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(\.[a-z0-9]+)*$`
	Type string `json:"type"`

	// Version of the event type specification (e.g. "1.0.0").
	// The major version must match the version suffix in Type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+.*$`
	Version string `json:"version"`

	// Description provides a human-readable summary of this event type.
	// +optional
	Description string `json:"description,omitempty"`

	// Specification contains the file ID reference from the file manager for
	// the optional JSON schema that describes the event payload.
	// +optional
	Specification string `json:"specification,omitempty"`

	// TODO: Add Category field (typed enum) - see backlog and ApiCategory implementation
}

// EventTypeStatus defines the observed state of EventType.
type EventTypeStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Active indicates whether this EventType is the active singleton for its type string.
	// When multiple EventTypes exist for the same type, only the oldest non-deleted one is active.
	Active bool `json:"active"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="The event type identifier"
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Indicates if this EventType is the active singleton"
// +kubebuilder:printcolumn:name="CreatedAt",type="date",JSONPath=".metadata.creationTimestamp",description="Creation timestamp"

// EventType is the Schema for the eventtypes API.
// It represents a singleton registry entry for a known event type, serving as the
// canonical reference that both EventExposure and EventSubscription point to.
type EventType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventTypeSpec   `json:"spec,omitempty"`
	Status EventTypeStatus `json:"status,omitempty"`
}

var _ types.Object = &EventType{}

func (r *EventType) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *EventType) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// EventTypeList contains a list of EventType
type EventTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventType `json:"items"`
}

var _ types.ObjectList = &EventTypeList{}

func (r *EventTypeList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&EventType{}, &EventTypeList{})
}
