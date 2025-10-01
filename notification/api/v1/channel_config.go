// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// AuthenticationConfig general configuration for authentication
type AuthenticationConfig interface {
	GetAuthentication() *Authentication
}

// MailConfig General configuration of a mailing channel
type MailConfig interface {
	GetSMTPHost() string
	GetSMTPPort() int
	GetFrom() string
	GetRecipients() []string
	GetCCRecipients() []string
	AuthenticationConfig
}

var _ MailConfig = &EmailConfig{}

// EmailConfig defines configuration for Email channel
type EmailConfig struct {

	// Recipients of this email
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=email
	Recipients []string `json:"recipients"`

	// CC Recipients of this email
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=email
	CCRecipients []string `json:"ccRecipients"`

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

func (ec *EmailConfig) GetSMTPHost() string {
	return ec.SMTPHost
}

func (ec *EmailConfig) GetSMTPPort() int {
	return ec.SMTPPort
}

func (ec *EmailConfig) GetFrom() string {
	return ec.From
}

func (ec *EmailConfig) GetRecipients() []string {
	return ec.Recipients
}

func (ec *EmailConfig) GetCCRecipients() []string {
	return ec.CCRecipients
}

func (ec *EmailConfig) GetAuthentication() *Authentication {
	return ec.Authentication
}

// ChatConfig general configuration for chat clients
type ChatConfig interface {
	GetWebhookURL() string
	AuthenticationConfig
}

var _ ChatConfig = &MsTeamsConfig{}

// MsTeamsConfig defines configuration for Microsoft Teams channel
type MsTeamsConfig struct {
	// Webhook URL for the Microsoft Teams channel
	// +kubebuilder:validation:Required
	WebhookURL string `json:"webhookUrl"`

	// Authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

func (msc *MsTeamsConfig) GetWebhookURL() string {
	return msc.WebhookURL
}

func (msc *MsTeamsConfig) GetAuthentication() *Authentication {
	return msc.Authentication
}

// CallbackConfig general config for custom callbacks - webhook etc
type CallbackConfig interface {
	GetURL() string
	GetMethod() string
	GetHeaders() map[string]string
	AuthenticationConfig
}

var _ CallbackConfig = &WebhookConfig{}

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

func (wc *WebhookConfig) GetURL() string {
	return wc.URL
}

func (wc *WebhookConfig) GetMethod() string {
	return wc.Method
}

func (wc *WebhookConfig) GetHeaders() map[string]string {
	return wc.Headers
}

func (wc *WebhookConfig) GetAuthentication() *Authentication {
	return wc.Authentication
}
