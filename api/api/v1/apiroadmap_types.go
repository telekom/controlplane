// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApiRoadmapSpec defines the desired state of ApiRoadmap
// +kubebuilder:validation:XValidation:rule="self.contents != ''",message="contents must not be empty"
// +kubebuilder:validation:XValidation:rule="self.hash != ''",message="hash must not be empty"
type ApiRoadmapSpec struct {
	// BasePath is the base path of the Api this roadmap describes.
	// The reconciler stays blocked until that Api exists.
	// +kubebuilder:validation:MinLength=1
	BasePath string `json:"basePath"`

	// Contents contains the file ID reference from the file manager.
	// The roadmap items are stored in the file manager as JSON.
	Contents string `json:"contents"`

	// Hash is the SHA-256 hash of the roadmap content for integrity verification.
	Hash string `json:"hash"`
}

// ApiRoadmapStatus defines the observed state of ApiRoadmap
type ApiRoadmapStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BasePath",type="string",JSONPath=".spec.basePath",description="The base path of the described API"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ApiRoadmap is the Schema for the apiroadmaps API.
// Once the described Api exists in the same namespace, the Api becomes the owner
// of the ApiRoadmap, so that deleting the Api also deletes the ApiRoadmap.
type ApiRoadmap struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApiRoadmapSpec   `json:"spec,omitempty"`
	Status ApiRoadmapStatus `json:"status,omitempty"`
}

var _ types.Object = &ApiRoadmap{}

func (r *ApiRoadmap) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *ApiRoadmap) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ApiRoadmapList contains a list of ApiRoadmap
type ApiRoadmapList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiRoadmap `json:"items"`
}

var _ types.ObjectList = &ApiRoadmapList{}

func (r *ApiRoadmapList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ApiRoadmap{}, &ApiRoadmapList{})
}
