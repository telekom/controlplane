// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MakeEventSpecificationName generates a name for the EventType resource
// based on the event type identifier of the EventSpecification.
// It replaces dots with hyphens and lowercases the result.
func MakeEventSpecificationName(eventSpec *EventSpecification) string {
	return strings.ToLower(strings.ReplaceAll(eventSpec.Spec.Type, ".", "-"))
}

// EventSpecificationSpec defines the desired state of EventSpecification.
type EventSpecificationSpec struct {
	// Type is the dot-separated event type identifier (e.g. "de.telekom.eni.quickstart.v1").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(\.[a-z0-9]+)*$`
	Type string `json:"type"`

	// Version of the event type specification (e.g. "1.0.0").
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
}

// EventSpecificationStatus defines the observed state of EventSpecification.
type EventSpecificationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// EventType references the EventType CR created from this specification.
	EventType types.ObjectRef `json:"eventType,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EventSpecification is the Schema for the eventspecifications API.
// It defines an event type's metadata and creates the corresponding EventType
// singleton in the event domain, analogous to how ApiSpecification creates Api resources.
type EventSpecification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventSpecificationSpec   `json:"spec,omitempty"`
	Status EventSpecificationStatus `json:"status,omitempty"`
}

var _ types.Object = &EventSpecification{}

func (r *EventSpecification) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *EventSpecification) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

type EventSpecificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventSpecification `json:"items"`
}

var _ types.ObjectList = &EventSpecificationList{}

func (r *EventSpecificationList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&EventSpecification{}, &EventSpecificationList{})
}
