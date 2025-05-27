// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoverSpec defines the desired state of Rover
type RoverSpec struct {
	Zone          string         `json:"zone"`
	Exposures     []Exposure     `json:"exposures,omitempty"`
	Subscriptions []Subscription `json:"subscriptions,omitempty"`
}

type Visibility string

const (
	VisibilityWorld      Visibility = "World"
	VisibilityZone       Visibility = "Zone"
	VisibilityEnterprise Visibility = "Enterprise"
)

func (v Visibility) String() string {
	return string(v)
}

type Type string

func (t Type) String() string {
	return string(t)
}

const (
	TypeApi   Type = "api"
	TypeEvent Type = "event"
)

type ApprovalStrategy string

const (
	ApprovalStrategyAuto     ApprovalStrategy = "Auto"
	ApprovalStrategySimple   ApprovalStrategy = "Simple"
	ApprovalStrategyFourEyes ApprovalStrategy = "FourEyes"
)

type Exposure struct {
	Api   *ApiExposure   `json:"api,omitempty"`
	Event *EventExposure `json:"event,omitempty"`
}

func (e *Exposure) Type() Type {
	if e.Api != nil {
		return TypeApi
	}
	if e.Event != nil {
		return TypeEvent
	}
	return ""
}

type Subscription struct {
	Api   *ApiSubscription   `json:"api,omitempty"`
	Event *EventSubscription `json:"event,omitempty"`
}

func (s *Subscription) Type() Type {
	if s.Api != nil {
		return TypeApi
	}
	if s.Event != nil {
		return TypeEvent
	}
	return ""
}

type ApiExposure struct {
	// BasePath is the base path of the API
	// +kubebuilder:validation:Pattern=`^/.*$`
	BasePath string `json:"basePath"`
	// +kubebuilder:validation:Format=uri
	Upstream string `json:"upstream"`
	// +kubebuilder:validation:Enum=World;Zone;Enterprise
	// +kubebuilder:default=Enterprise
	Visibility Visibility `json:"visibility"`
	// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
	// +kubebuilder:default=Auto
	Approval ApprovalStrategy `json:"approval"`
}

type EventExposure struct {
	EventType string `json:"eventType"`
}

type ApiSubscription struct {
	// BasePath is the base path of the API
	// +kubebuilder:validation:Pattern=^/.*$
	BasePath string `json:"basePath"`
	// Organization is the organization that owns the API
	// Defaults to local organization
	Organization string   `json:"organization,omitempty"`
	OAuth2Scopes []string `json:"oauth2Scopes,omitempty"`
}

type EventSubscription struct {
	EventType string `json:"eventType"`
}

// RoverStatus defines the observed state of Rover
type RoverStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	Application      *types.ObjectRef  `json:"application,omitempty"`
	ApiSubscriptions []types.ObjectRef `json:"apiSubscriptions,omitempty"`
	ApiExposures     []types.ObjectRef `json:"apiExposures,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Rover is the Schema for the rovers API
type Rover struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RoverSpec   `json:"spec,omitempty"`
	Status RoverStatus `json:"status,omitempty"`
}

var _ types.Object = &Rover{}

func (r *Rover) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Rover) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

//+kubebuilder:object:root=true

// RoverList contains a list of Rover
type RoverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rover `json:"items"`
}

var _ types.ObjectList = &RoverList{}

func (r *RoverList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items = append(items, &r.Items[i])
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Rover{}, &RoverList{})
}
