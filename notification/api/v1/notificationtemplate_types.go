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

// NotificationTemplateSpec defines the desired state of NotificationTemplate
type NotificationTemplateSpec struct {
	// Purpose defines the purpose of this notification template
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	Purpose string `json:"purpose"`

	// ChannelType defines which notification channel type this template is for
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Email;MsTeams;Webhook
	ChannelType string `json:"channelType"`

	// Subject line template for Email notifications
	// +optional
	SubjectTemplate string `json:"subjectTemplate,omitempty"`

	// Template content with placeholders for variables
	// +kubebuilder:validation:Required
	Template string `json:"template"`

	// Schema defines the expected properties for rendering the template
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Schema runtime.RawExtension `json:"schema,omitempty"`
}

// NotificationTemplateStatus defines the observed state of NotificationTemplate.
type NotificationTemplateStatus struct {
	// Conditions represent the latest available observations of the Rover's state
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NotificationTemplate is the Schema for the notificationtemplates API
type NotificationTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of NotificationTemplate
	// +required
	Spec NotificationTemplateSpec `json:"spec"`

	// status defines the observed state of NotificationTemplate
	// +optional
	Status NotificationTemplateStatus `json:"status,omitempty,omitzero"`
}

var _ types.Object = &NotificationTemplate{}

func (r *NotificationTemplate) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *NotificationTemplate) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// NotificationTemplateList contains a list of NotificationTemplate
type NotificationTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NotificationTemplate `json:"items"`
}

var _ types.ObjectList = &NotificationTemplateList{}

func (r *NotificationTemplateList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items = append(items, &r.Items[i])
	}
	return items
}

func init() {
	SchemeBuilder.Register(&NotificationTemplate{}, &NotificationTemplateList{})
}
