// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ApiSpecificationSpec struct {
	Specification string `json:"specification"`
}

type ApiSpecificationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// API reference
	Api types.ObjectRef `json:"api,omitempty"`
	// Base path of the API
	BasePath string `json:"basePath,omitempty"`
	// Category of the API
	Category string `json:"category,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ApiSpecification is the Schema for the apispecifications API
// +kubebuilder:pruning:PreserveUnknownFields
type ApiSpecification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApiSpecificationSpec   `json:"spec,omitempty"`
	Status ApiSpecificationStatus `json:"status,omitempty"`
}

var _ types.Object = &ApiSpecification{}

func (r *ApiSpecification) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *ApiSpecification) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

//+kubebuilder:object:root=true

type ApiSpecificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiSpecification `json:"items"`
}

var _ types.ObjectList = &ApiSpecificationList{}

func (r *ApiSpecificationList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items = append(items, &r.Items[i])
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ApiSpecification{}, &ApiSpecificationList{})
}
