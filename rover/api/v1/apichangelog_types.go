// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApiChangelogItem represents a single changelog entry
// This struct is used for API/mapping layer only and is NOT stored in the CRD
// The actual items are stored in file-manager as JSON
type ApiChangelogItem struct {
	// Date of the changelog entry (e.g., "2024-03-15")
	Date string `json:"date"`

	// Version identifier for this changelog entry (e.g., "v1.2.0")
	Version string `json:"version"`

	// Description provides detailed information about the changes
	Description string `json:"description"`

	// VersionUrl is an optional URL link for the version release notes
	VersionUrl string `json:"versionUrl,omitempty"`
}

// ApiChangelogSpec defines the desired state of ApiChangelog
// +kubebuilder:validation:XValidation:rule="self.contents != ''",message="contents must not be empty"
// +kubebuilder:validation:XValidation:rule="self.hash != ''",message="hash must not be empty"
type ApiChangelogSpec struct {
	// SpecificationRef is a reference to the ApiSpecification this changelog describes
	// The referenced specification may or may not exist yet
	// +kubebuilder:validation:Required
	SpecificationRef types.TypedObjectRef `json:"specificationRef"`

	// Contents contains the file ID reference from the file manager
	// The actual changelog items array is stored in file-manager as JSON
	// +kubebuilder:validation:Required
	Contents string `json:"contents"`

	// Hash is the SHA-256 hash of the changelog content for integrity verification
	// +kubebuilder:validation:Required
	Hash string `json:"hash"`
}

// ApiChangelogStatus defines the observed state of ApiChangelog
type ApiChangelogStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ApiChangelog is the Schema for the apichangelogs API
// The ApiChangelog name is the normalized basePath with major version removed
// Example: basePath "/eni/my-api/v1" → changelog name "eni-my-api"
// +kubebuilder:pruning:PreserveUnknownFields
type ApiChangelog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApiChangelogSpec   `json:"spec,omitempty"`
	Status ApiChangelogStatus `json:"status,omitempty"`
}

var _ types.Object = &ApiChangelog{}

func (r *ApiChangelog) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *ApiChangelog) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

//+kubebuilder:object:root=true

// ApiChangelogList contains a list of ApiChangelog
type ApiChangelogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiChangelog `json:"items"`
}

var _ types.ObjectList = &ApiChangelogList{}

func (r *ApiChangelogList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ApiChangelog{}, &ApiChangelogList{})
}
