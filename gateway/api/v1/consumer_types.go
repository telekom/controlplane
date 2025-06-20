// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConsumerSpec defines the desired state of Consumer
type ConsumerSpec struct {
	Realm types.ObjectRef `json:"realm"`
	Name  string          `json:"name"`
}

// ConsumerStatus defines the observed state of Consumer
type ConsumerStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions          []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	KongConsumerId      string             `json:"kongConsumerId"`
	KongConsumerGroupId string             `json:"kongConsumerGroupId"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Consumer is the Schema for the consumers API
type Consumer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConsumerSpec   `json:"spec,omitempty"`
	Status ConsumerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConsumerList contains a list of Consumer
type ConsumerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Consumer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Consumer{}, &ConsumerList{})
}

var _ types.Object = &Consumer{}

func (c *Consumer) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

func (c *Consumer) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&c.Status.Conditions, condition)
}

func (c *ConsumerList) GetItems() []types.Object {
	items := make([]types.Object, len(c.Items))
	for i := range c.Items {
		items[i] = &c.Items[i]
	}
	return items
}
