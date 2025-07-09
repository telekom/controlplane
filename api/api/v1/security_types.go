// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// Security defines the security configuration for the Rover
// Security is optional, but if provided, exactly one of m2m or h2m must be set
type Security struct {
	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *Machine2MachineAuthentication `json:"m2m,omitempty"`
}

// SubscriberSecurity defines the security configuration for the Rover
// SubscriberSecurity is optional, but if provided, exactly one of m2m or h2m must be set
type SubscriberSecurity struct {
	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *SubscriberMachine2MachineAuthentication `json:"m2m,omitempty"`
}

// Machine2MachineAuthentication defines the authentication methods for machine-to-machine communication
// Either externalIDP, basic, or only scopes can be provided
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.externalIDP) ? (!has(self.basic)) : true)", message="ExternalIDP and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || has(self.externalIDP) || has(self.basic) || has(self.scopes)", message="At least one of externalIDP, basic, or scopes must be provided"
type Machine2MachineAuthentication struct {
	// ExternalIDP defines external identity provider configuration
	// +kubebuilder:validation:Optional
	ExternalIDP *ExternalIdentityProvider `json:"externalIDP,omitempty"`
	// Basic defines basic authentication configuration
	// +kubebuilder:validation:Optional
	Basic *BasicAuthCredentials `json:"basic,omitempty"`
	// Scopes defines additional OAuth2 scopes that are added to the LMS token
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=10
	Scopes []string `json:"scopes,omitempty"`
}

// SubscriberMachine2MachineAuthentication defines the authentication methods for machine-to-machine communication for subscribers
// Either client, basic, or only scopes can be provided
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.client) ? (!has(self.basic)) : true)", message="Client and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || has(self.client) || has(self.basic) || has(self.scopes)", message="At least one of client, basic, or scopes must be provided"
type SubscriberMachine2MachineAuthentication struct {
	// Client defines client credentials for OAuth2
	// +kubebuilder:validation:Optional
	Client *OAuth2ClientCredentials `json:"client,omitempty"`
	// Basic defines basic authentication configuration
	// +kubebuilder:validation:Optional
	Basic *BasicAuthCredentials `json:"basic,omitempty"`
	// Scopes defines additional OAuth2 scopes that are added to the LMS token
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=10
	Scopes []string `json:"scopes,omitempty"`
}

// ExternalIdentityProvider defines configuration for using an external identity provider
// +kubebuilder:validation:XValidation:rule="self == null || has(self.basic) != has(self.client)", message="Only one of basic or client credentials can be provided (XOR relationship)"
type ExternalIdentityProvider struct {
	// TokenEndpoint is the URL for the OAuth2 token endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	TokenEndpoint string `json:"tokenEndpoint"`

	// TokenRequest is the type of token request, "body" or "header"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=body;header
	TokenRequest string `json:"tokenRequest,omitempty"`

	// GrantType defines the OAuth2 grant type to use for the token request
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=client_credentials;authorization_code;password
	GrantType string `json:"grantType,omitempty"`

	// Basic defines basic auth credentials for the OAuth2 token request
	Basic *BasicAuthCredentials `json:"basic,omitempty"`
	// Client defines client credentials for the OAuth2 token request
	Client *OAuth2ClientCredentials `json:"client,omitempty"`
}

// BasicAuthCredentials defines username/password credentials
type BasicAuthCredentials struct {
	// Username for basic authentication
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Username string `json:"username"`
	// Password for basic authentication
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Password string `json:"password"`
}

// OAuth2ClientCredentials defines client credentials for OAuth2
type OAuth2ClientCredentials struct {
	// ClientId identifies the client for OAuth2 client credentials flow
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientId string `json:"clientId"`
	// ClientSecret is the secret associated with the client ID
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientSecret string `json:"clientSecret"`
	// Scopes defines the OAuth2 scopes to request in the token
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`
}
