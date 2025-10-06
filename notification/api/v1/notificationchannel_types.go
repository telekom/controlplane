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
	// +kubebuilder:validation:Pattern=`^https?://[^\s/$.?#].[^\s]*$`
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
	// Mail configuration, required if Type is Mail
	// +optional
	Email *EmailConfig `json:"email,omitempty"`

	// Chat configuration, required if Type is Chat
	// +optional
	MsTeams *MsTeamsConfig `json:"msTeams,omitempty"`

	// Callback configuration, required if Type is Callback
	// +optional
	Webhook *WebhookConfig `json:"webhook,omitempty"`

	// A set of purposes this channel ignores
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +listType=set
	Ignore []string `json:"ignore,omitempty"`
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

func (r *NotificationChannel) NotificationType() NotificationType {
	if r.Spec.Email != nil {
		return NotificationTypeMail
	}
	if r.Spec.MsTeams != nil {
		return NotificationTypeChat
	}
	if r.Spec.Webhook != nil {
		return NotificationTypeCallback
	}

	// default value - should never happen
	return ""
}

type NotificationType string

const (
	NotificationTypeMail     NotificationType = "Mail"
	NotificationTypeChat     NotificationType = "Chat"
	NotificationTypeCallback NotificationType = "Callback"
)

// EmailString wraps a string and applies email validation
// +kubebuilder:validation:Format=email
type EmailString string

// EmailConfig defines configuration for Email channel
type EmailConfig struct {

	// Recipients of this email
	// +kubebuilder:validation:Required
	Recipients []EmailString `json:"recipients"`

	// CC Recipients of this email
	// +kubebuilder:validation:Optional
	CCRecipients []EmailString `json:"ccRecipients"`

	// SMTP server host
	// +kubebuilder:validation:Optional
	SMTPHost string `json:"smtpHost"`

	// SMTP server port
	// +kubebuilder:validation:Optional
	SMTPPort int `json:"smtpPort"`

	// From email address
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=email
	From string `json:"from"`

	// Authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

// GetRecipients - func that returns the recipients for this email notification
// The reason it is written like this is that a conversion between EmailString and string needs to be done here
// The EmailString type is required to have correct validation (otherwise kubebuilder generates CRDs where the whole slice format is email
// The return type is a slice of strings so there is no dependency between the model and the notification adapters
func (e *EmailConfig) GetRecipients() []string {
	result := make([]string, len(e.Recipients))
	for i, r := range e.Recipients {
		result[i] = string(r)
	}
	return result
}

func (e *EmailConfig) GetCCRecipients() []string {
	result := make([]string, len(e.CCRecipients))
	for i, r := range e.CCRecipients {
		result[i] = string(r)
	}
	return result
}

func (e *EmailConfig) GetSMTPHost() string {
	return e.SMTPHost
}

func (e *EmailConfig) GetSMTPPort() int {
	return e.SMTPPort
}

func (e *EmailConfig) GetFrom() string {
	return e.From
}

// IsNotificationConfig - make sure the generic adapter will accept this config
func (e *EmailConfig) IsNotificationConfig() {}

// MsTeamsConfig defines configuration for Microsoft Teams channel
type MsTeamsConfig struct {
	// Webhook URL for the Microsoft Teams channel
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://[^\s/$.?#].[^\s]*$`
	WebhookURL string `json:"webhookUrl"`

	// Authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

func (m *MsTeamsConfig) GetWebhookURL() string {
	return m.WebhookURL
}

// IsNotificationConfig - make sure the generic adapter will accept this config
func (m *MsTeamsConfig) IsNotificationConfig() {}

// WebhookConfig defines configuration for generic webhook channel
type WebhookConfig struct {
	// URL of the webhook endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://[^\s/$.?#].[^\s]*$`
	URL string `json:"url"`

	// HTTP method to use
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=POST
	// +kubebuilder:default=POST
	Method string `json:"method"`

	// Headers to include in the request
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// Authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

func (w *WebhookConfig) GetURL() string {
	return w.URL
}

func (w *WebhookConfig) GetMethod() string {
	return w.Method
}

func (w *WebhookConfig) GetHeaders() map[string]string {
	return w.Headers
}

// IsNotificationConfig - make sure the generic adapter will accept this config
func (w *WebhookConfig) IsNotificationConfig() {}
