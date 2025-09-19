// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var BasePathLabelKey = config.BuildLabelKey("basepath")

// ApiSpec defines the desired state of Api
type ApiSpec struct {
	Version      string   `json:"version"`
	BasePath     string   `json:"basePath"`
	Category     string   `json:"category"`
	Oauth2Scopes []string `json:"oauth2Scopes,omitempty"`
	XVendor      bool     `json:"xVendor"`
}

// ApiStatus defines the observed state of Api
type ApiStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	Active     bool               `json:"active"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Api is the Schema for the apis API
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Indicates if the API is active"
type Api struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApiSpec   `json:"spec,omitempty"`
	Status ApiStatus `json:"status,omitempty"`
}

var _ types.Object = &Api{}

func (r *Api) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Api) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ApiList contains a list of Api
type ApiList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Api `json:"items"`
}

var _ types.ObjectList = &ApiList{}

func (r *ApiList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Api{}, &ApiList{})
}
