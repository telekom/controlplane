// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ApprovalStrategy string

const (
	ApprovalStrategyAuto     ApprovalStrategy = "Auto"
	ApprovalStrategySimple   ApprovalStrategy = "Simple"
	ApprovalStrategyFourEyes ApprovalStrategy = "FourEyes"
)

// ApiExposureSpec defines the desired state of ApiExposure
type ApiExposureSpec struct {
	ApiBasePath string     `json:"apiBasePath"`
	Upstreams   []Upstream `json:"upstreams"`
	// +kubebuilder:validation:Enum=World;Zone;Enterprise
	Visibility Visibility `json:"visibility"`
	// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
	// +kubebuilder:default=Auto
	Approval ApprovalStrategy `json:"approval"`
	Zone     ctypes.ObjectRef `json:"zone"`

	Security *Security `json:"security,omitempty"`
}

type Upstream struct {
	Url    string `json:"url"`
	Weight int    `json:"weight,omitempty"`
}

type Visibility string

const (
	VisibilityWorld      Visibility = "World"
	VisibilityZone       Visibility = "Zone"
	VisibilityEnterprise Visibility = "Enterprise"
)

// ApiExposureStatus defines the observed state of ApiExposure
type ApiExposureStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	Active bool              `json:"active"`
	Route  *ctypes.ObjectRef `json:"route,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ApiExposure is the Schema for the apiexposures API
// +kubebuilder:printcolumn:name="Active",type="boolean",JSONPath=".status.active",description="Indicates if the API is active"
type ApiExposure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApiExposureSpec   `json:"spec,omitempty"`
	Status ApiExposureStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &ApiExposure{}

func (r *ApiExposure) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *ApiExposure) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ApiExposureList contains a list of ApiExposure
type ApiExposureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiExposure `json:"items"`
}

var _ ctypes.ObjectList = &ApiExposureList{}

func (r *ApiExposureList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ApiExposure{}, &ApiExposureList{})
}
