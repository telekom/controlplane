// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NoneAuth represents no authentication
type NoneAuth struct{}

type Oauth2Auth struct {
	// TokenURL is the URL to obtain the OAuth2 token
	// +kubebuilder:validation:Required
	TokenURL string `json:"tokenUrl"`

	// ClientID is the OAuth2 client ID
	// +kubebuilder:validation:Required
	ClientID string `json:"clientId"`

	// ClientSecret is the OAuth2 client secret
	// +kubebuilder:validation:Required
	ClientSecret string `json:"clientSecret"`

	// Scopes are the OAuth2 scopes
	// +optional
	Scopes []string `json:"scopes,omitempty"`
}

// Authentication represents authentication configuration for a notification channel
type Authentication struct {
	None   *NoneAuth   `json:"none,omitempty"`
	Oauth2 *Oauth2Auth `json:"oauth2,omitempty"`
}

// NotificationChannelSpec defines the desired state of NotificationChannel
// +kubebuilder:validation:XValidation:rule="(has(self.email) ? 1 : 0) + (has(self.msTeams) ? 1 : 0) + (has(self.webhook) ? 1 : 0) == 1",message="Exactly one of email, msTeams, webhook must be specified"
type NotificationChannelSpec struct {
	// Email configuration, required if Type is Email
	// +optional
	Email *EmailConfig `json:"email,omitempty"`

	// MsTeams configuration, required if Type is MsTeams
	// +optional
	MsTeams *MsTeamsConfig `json:"msTeams,omitempty"`

	// Webhook configuration, required if Type is Webhook
	// +optional
	Webhook *WebhookConfig `json:"webhook,omitempty"`

	// A set of purposes this channel ignores
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +listType=set
	Ignore []string `json:"ignore,omitempty"`
}

// EmailConfig defines configuration for Email channel
type EmailConfig struct {
	// SMTP server host
	// +kubebuilder:validation:Required
	SMTPHost string `json:"smtpHost"`

	// SMTP server port
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	SMTPPort int `json:"smtpPort"`

	// From email address
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=email
	From string `json:"from"`

	// Authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

// MsTeamsConfig defines configuration for Microsoft Teams channel
type MsTeamsConfig struct {
	// Webhook URL for the Microsoft Teams channel
	// +kubebuilder:validation:Required
	WebhookURL string `json:"webhookUrl"`

	// Authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

// WebhookConfig defines configuration for generic webhook channel
type WebhookConfig struct {
	// URL of the webhook endpoint
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// HTTP method to use
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=POST;PUT
	// +kubebuilder:default=POST
	Method string `json:"method"`

	// Headers to include in the request
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// Authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

// NotificationChannelStatus defines the observed state of NotificationChannel.
type NotificationChannelStatus struct {
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

// NotificationChannel is the Schema for the notificationchannels API
type NotificationChannel struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of NotificationChannel
	// +required
	Spec NotificationChannelSpec `json:"spec"`

	// status defines the observed state of NotificationChannel
	// +optional
	Status NotificationChannelStatus `json:"status,omitempty,omitzero"`
}

var _ types.Object = &NotificationChannel{}

func (r *NotificationChannel) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *NotificationChannel) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// NotificationChannelList contains a list of NotificationChannel
type NotificationChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NotificationChannel `json:"items"`
}

var _ types.ObjectList = &NotificationChannelList{}

func (r *NotificationChannelList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items = append(items, &r.Items[i])
	}
	return items
}

func init() {
	SchemeBuilder.Register(&NotificationChannel{}, &NotificationChannelList{})
}
