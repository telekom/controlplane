// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConsumeRouteSpec defines the desired state of ConsumeRoute
type ConsumeRouteSpec struct {
	Route        types.ObjectRef   `json:"route"`
	ConsumerName string            `json:"consumerName"`
	Security     *ConsumerSecurity `json:"security,omitempty"`
}

func (c *ConsumeRoute) HasM2M() bool {
	if c.Spec.Security == nil {
		return false
	}
	return c.Spec.Security.M2M != nil
}

func (c *ConsumeRoute) HasM2MClient() bool {
	if !c.HasM2M() {
		return false
	}
	return c.Spec.Security.M2M.Client != nil
}

func (c *ConsumeRoute) HasM2MBasic() bool {
	if !c.HasM2M() {
		return false
	}
	return c.Spec.Security.M2M.Basic != nil
}

// ConsumeRouteStatus defines the observed state of ConsumeRoute
type ConsumeRouteStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ConsumeRoute is the Schema for the consumeroutes API
type ConsumeRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConsumeRouteSpec   `json:"spec,omitempty"`
	Status ConsumeRouteStatus `json:"status,omitempty"`
}

var _ types.Object = &ConsumeRoute{}

func (c *ConsumeRoute) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

func (c *ConsumeRoute) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&c.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ConsumeRouteList contains a list of ConsumeRoute
type ConsumeRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConsumeRoute `json:"items"`
}

var _ types.ObjectList = &ConsumeRouteList{}

func (c *ConsumeRouteList) GetItems() []types.Object {
	items := make([]types.Object, len(c.Items))
	for i := range c.Items {
		items[i] = &c.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ConsumeRoute{}, &ConsumeRouteList{})
}
