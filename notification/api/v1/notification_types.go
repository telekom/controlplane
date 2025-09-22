// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

type SenderType string

const (
	SenderTypeUser   SenderType = "User"
	SenderTypeSystem SenderType = "System"
)

type Sender struct {
	// Type defines the type of the sender
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=User;System
	Type SenderType `json:"type"`

	// Name defines the name of the sender
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Name string `json:"name,omitempty"`
}

// NotificationSpec defines the desired state of Notification
type NotificationSpec struct {
	// Purpose defines the purpose of the notification.
	// It is used to select the notification template.
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	Purpose string `json:"purpose"`

	// Sender contains the information about the entity that is sending the notification
	// +kubebuilder:validation:Required
	Sender Sender `json:"sender"`

	// Channels defines the channels to send the notification to.
	// +kubebuilder:validation:UniqueItems=true
	// +listType=set
	Channels []types.ObjectRef `json:"channels,omitempty"`

	// Properties contains the properties that are used to render the notification template
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:default={ }
	// +kubebuilder:validation:XValidation:rule="!has(self) || (has(self) && self.size() <= 1024)",message="properties must not exceed 1024 bytes"
	Properties runtime.RawExtension `json:"properties"`
}

// NotificationStatus defines the observed state of Notification.
type NotificationStatus struct {
	// Conditions represent the latest available observations of the Rover's state
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// States collects the states of the notification per channel
	States map[string]SendState `json:"states,omitempty"`
}

type SendState struct {
	Timestamp metav1.Time `json:"timestamp"`

	// Sent indicates whether the notification was sent successfully
	Sent bool `json:"sent"`

	// ErrorMessage contains the error message if the notification failed to send
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Notification is the Schema for the notifications API
type Notification struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Notification
	// +required
	Spec NotificationSpec `json:"spec"`

	// status defines the observed state of Notification
	// +optional
	Status NotificationStatus `json:"status,omitempty,omitzero"`
}

var _ types.Object = &Notification{}

func (r *Notification) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Notification) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// NotificationList contains a list of Notification
type NotificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Notification `json:"items"`
}

var _ types.ObjectList = &NotificationList{}

func (r *NotificationList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items = append(items, &r.Items[i])
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Notification{}, &NotificationList{})
}
