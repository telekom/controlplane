// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// Security defines the security configuration for the gateway
// Security is optional, but if provided, exactly one of m2m or h2m must be set
// +kubebuilder:validation:XValidation:rule="self == null || has(self.m2m) != has(self.h2m)", message="Only one of m2m or h2m authentication can be provided (XOR relationship)"
type Security struct {
	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *Machine2MachineAuthentication `json:"m2m,omitempty"`
	// H2M defines human-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	H2M *Human2MachineAuthentication `json:"h2m,omitempty"`
}

// Security defines the security configuration for the Rover
// Security is optional, but if provided, exactly one of m2m or h2m must be set
// +kubebuilder:validation:XValidation:rule="self == null || has(self.m2m) != has(self.h2m)", message="Only one of m2m or h2m authentication can be provided (XOR relationship)"
type SubscriberSecurity struct {
	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *SubscriberMachine2MachineAuthentication `json:"m2m,omitempty"`
	// H2M defines human-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	H2M *SubscriberHuman2MachineAuthentication `json:"h2m,omitempty"`
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

// SubscriberHuman2MachineAuthentication defines the authentication methods for human-to-machine communication for subscribers
type SubscriberHuman2MachineAuthentication struct {
	// +kubebuilder:validation:Optional
	// Future authentication methods will be added here
}

// Machine2MachineAuthentication defines the authentication methods for machine-to-machine communication
// Either externalIDP, basic, or only scopes can be provided
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.externalIDP) ? (!has(self.basic)) : true)", message="ExternalIDP and basic authentication cannot be used together"
type Machine2MachineAuthentication struct {
	// ExternalIDP defines external identity provider configuration
	// +kubebuilder:validation:Optional
	ExternalIDP *ExternalIdentityProvider `json:"externalIDP,omitempty"`

	// Client defines client credentials for OAuth2
	Client OAuth2ClientCredentials `json:"client,omitempty"`

	// Basic defines basic authentication configuration
	// +kubebuilder:validation:Optional
	Basic *BasicAuthCredentials `json:"basic,omitempty"`
	// Scopes defines additional OAuth2 scopes that are added to the LMS token
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=10
	Scopes []string `json:"scopes,omitempty"`
}

// Human2MachineAuthentication defines the authentication methods for human-to-machine communication
type Human2MachineAuthentication struct {
	// +kubebuilder:validation:Optional
	// Future authentication methods will be added here
}

// ExternalIdentityProvider defines the external identity provider configuration
type ExternalIdentityProvider struct {
	// TokenEndpoint is the URL for the OAuth2 token endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	TokenEndpoint string `json:"tokenEndpoint"`

	// TokenRequest is the type of token request, "body" or "header"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=body;header
	TokenRequest string `json:"tokenRequest,omitempty"`
	// GrantType is the grant type for the external IDP authentication
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=client_credentials;authorization_code;password
	GrantType string `json:"grantType,omitempty"`

	// Client defines client credentials for OAuth2 for the external IDP authentication
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
