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
	Visibility Visibility       `json:"visibility"`
	Approval   Approval         `json:"approval"`
	Zone       ctypes.ObjectRef `json:"zone"`

	Traffic Traffic `json:"traffic"`

	Transformation *Transformation `json:"transformation,omitempty"`

	Security *Security `json:"security,omitempty"`
}

type Approval struct {
	// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
	// +kubebuilder:default=Auto
	Strategy ApprovalStrategy `json:"strategy"`
	// TrustedTeams identifies teams that are trusted for approving this API
	// Per default your own team is trusted
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	TrustedTeams []string `json:"trustedTeams,omitempty"`
}

func (exposure *ApiExposure) HasExternalIdp() bool {

	if exposure.Spec.Security == nil {
		return false
	}
	if exposure.Spec.Security.M2M == nil {
		return false
	}
	if exposure.Spec.Security.M2M.ExternalIDP == nil {
		return false
	}

	return exposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint != ""

}

func (a *ApiExposure) HasFailover() bool {
	return a.Spec.Traffic.Failover != nil
}

func (a *ApiExposure) HasRateLimit() bool {
	return a.Spec.Traffic.RateLimit != nil
}

func (a *ApiExposure) HasM2M() bool {
	if a.Spec.Security == nil {
		return false
	}

	return a.Spec.Security.M2M != nil
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

	Active        bool              `json:"active"`
	Route         *ctypes.ObjectRef `json:"route,omitempty"`
	FailoverRoute *ctypes.ObjectRef `json:"failoverRoute,omitempty"`
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
