// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceType represents the type of resource the roadmap is associated with
// +kubebuilder:validation:Enum=API;Event
type ResourceType string

const (
	// ResourceTypeAPI indicates the roadmap is for an API resource
	ResourceTypeAPI ResourceType = "API"
	// ResourceTypeEvent indicates the roadmap is for an Event resource
	ResourceTypeEvent ResourceType = "Event"
)

// RoadmapItem represents a single timeline entry in the roadmap
// This struct is used for API/mapping layer only and is NOT stored in the CRD
// The actual items are stored in file-manager as JSON
type RoadmapItem struct {
	// Date of the roadmap item (e.g., "Q1 2024", "2024-03-15")
	Date string `json:"date"`

	// Title of the roadmap item
	Title string `json:"title"`

	// Description provides detailed information about the roadmap item
	Description string `json:"description"`

	// TitleUrl is an optional URL link for the roadmap item title
	TitleUrl string `json:"titleUrl,omitempty"`
}

// RoadmapSpec defines the desired state of Roadmap
type RoadmapSpec struct {
	// ResourceName is the generic identifier for the resource
	// For APIs: the basePath (e.g., "/eni/my-api/v1")
	// For Events: the event type name (e.g., "de.telekom.eni.myevent.v1")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	ResourceName string `json:"resourceName"`

	// ResourceType distinguishes the type of resource
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=API;Event
	ResourceType ResourceType `json:"resourceType"`

	// Roadmap contains the file ID reference from the file manager
	// The actual roadmap items array is stored in file-manager as JSON
	// +kubebuilder:validation:Required
	Roadmap string `json:"roadmap"`

	// Hash is the SHA-256 hash of the roadmap content for integrity verification
	// +kubebuilder:validation:Required
	Hash string `json:"hash"`
}

// RoadmapStatus defines the observed state of Roadmap
type RoadmapStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Roadmap is the Schema for the roadmaps API
// +kubebuilder:pruning:PreserveUnknownFields
type Roadmap struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoadmapSpec   `json:"spec,omitempty"`
	Status RoadmapStatus `json:"status,omitempty"`
}

var _ types.Object = &Roadmap{}

func (r *Roadmap) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Roadmap) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

//+kubebuilder:object:root=true

// RoadmapList contains a list of Roadmap
type RoadmapList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Roadmap `json:"items"`
}

var _ types.ObjectList = &RoadmapList{}

func (r *RoadmapList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Roadmap{}, &RoadmapList{})
}
