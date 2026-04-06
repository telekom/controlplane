// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=API;Event
type ResourceType string

const (
	ResourceTypeAPI   ResourceType = "API"
	ResourceTypeEvent ResourceType = "Event"
)

// ChangelogItem is used in API/mapping layer only. Items are stored in file-manager as JSON.
type ChangelogItem struct {
	Date        string `json:"date"`
	Version     string `json:"version"`
	Description string `json:"description"`
	VersionUrl  string `json:"versionUrl,omitempty"`
}

type ChangelogSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	ResourceName string `json:"resourceName"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=API;Event
	ResourceType ResourceType `json:"resourceType"`

	// File ID reference in file-manager
	// +kubebuilder:validation:Required
	Changelog string `json:"changelog"`

	// +kubebuilder:validation:Required
	Hash string `json:"hash"`
}

type ChangelogStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:pruning:PreserveUnknownFields
type Changelog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChangelogSpec   `json:"spec,omitempty"`
	Status ChangelogStatus `json:"status,omitempty"`
}

var _ types.Object = &Changelog{}

func (r *Changelog) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Changelog) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

//+kubebuilder:object:root=true

type ChangelogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Changelog `json:"items"`
}

var _ types.ObjectList = &ChangelogList{}

func (r *ChangelogList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Changelog{}, &ChangelogList{})
}
