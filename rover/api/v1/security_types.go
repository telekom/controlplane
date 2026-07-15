// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// TokenRequestMethod defines the token endpoint authentication method (RFC 7591).
// +kubebuilder:validation:Enum=client_secret_basic;client_secret_post
type TokenRequestMethod string

const (
	TokenRequestClientSecretBasic TokenRequestMethod = "client_secret_basic"
	TokenRequestClientSecretPost  TokenRequestMethod = "client_secret_post"
)

// GrantType defines the OAuth2 grant type for external IDP token requests.
// +kubebuilder:validation:Enum=client_credentials;authorization_code;password
type GrantType string

const (
	GrantTypeClientCredentials GrantType = "client_credentials"
	GrantTypeAuthorizationCode GrantType = "authorization_code"
	GrantTypePassword          GrantType = "password"
)

// Security defines the security configuration for the Rover
// Security is optional, but if provided, exactly one of m2m or h2m must be set
type Security struct {
	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *Machine2MachineAuthentication `json:"m2m,omitempty"`
}

// ClaimValueFrom is a predefined source that the Control Plane (or Jumper at runtime)
// resolves into the claim value.
// +kubebuilder:validation:Enum=ProviderClientId;ConsumerClientId;BasePath
type ClaimValueFrom string

const (
	ClaimValueFromProviderClientId ClaimValueFrom = "ProviderClientId"
	ClaimValueFromConsumerClientId ClaimValueFrom = "ConsumerClientId"
	ClaimValueFromBasePath         ClaimValueFrom = "BasePath"
)

// Claims defines the set of token claims. Currently only audience (aud) is supported.
type Claims struct {
	// Aud defines the audience claim
	// +kubebuilder:validation:Optional
	Aud *Claim `json:"aud,omitempty"`
}

// Claim defines a single claim value. Exactly one of value or valueFrom must be set.
// +kubebuilder:validation:XValidation:rule="has(self.value) != has(self.valueFrom)",message="exactly one of value or valueFrom must be set"
type Claim struct {
	// Value is a literal claim value provided by the user
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=256
	Value string `json:"value,omitempty"`
	// ValueFrom is a predefined source resolved into the claim value
	// +kubebuilder:validation:Optional
	ValueFrom ClaimValueFrom `json:"valueFrom,omitempty"`
}

// Security defines the security configuration for the Rover
// Security is optional, but if provided, exactly one of m2m or h2m must be set
type SubscriberSecurity struct {
	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *SubscriberMachine2MachineAuthentication `json:"m2m,omitempty"`
}

// Machine2MachineAuthentication defines the authentication methods for machine-to-machine communication
// Either externalIDP, basic, or only scopes can be provided
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.externalIDP) ? (!has(self.basic)) : true)", message="ExternalIDP and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.scopes) ? (!has(self.basic)) : true)", message="Scopes and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || has(self.externalIDP) || has(self.basic) || has(self.scopes) || has(self.claims)", message="At least one of externalIDP, basic, scopes, or claims must be provided"
// +kubebuilder:validation:XValidation:rule="self == null || !has(self.claims) || (!has(self.externalIDP) && !has(self.basic))", message="Claims require the platform-managed token and cannot be used with an external IDP or basic authentication"
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
	// +kubebuilder:validation:items:MaxLength=256
	Scopes []string `json:"scopes,omitempty"`
	// Claims defines token claims that must be present in the token reaching the upstream.
	// Only valid on the platform-managed LMS token.
	// +kubebuilder:validation:Optional
	Claims *Claims `json:"claims,omitempty"`
}

// SubscriberMachine2MachineAuthentication defines the authentication methods for machine-to-machine communication for subscribers
// Either client, basic, or only scopes can be provided
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.client) ? (!has(self.basic)) : true)", message="Client and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || has(self.client) || has(self.basic) || has(self.scopes) || has(self.claims)", message="At least one of client, basic, scopes, or claims must be provided"
// +kubebuilder:validation:XValidation:rule="self == null || !has(self.claims) || (!has(self.client) && !has(self.basic))", message="Claims require the platform-managed token and cannot be used with an external IDP or basic authentication"
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
	// +kubebuilder:validation:items:MaxLength=256
	Scopes []string `json:"scopes,omitempty"`
	// Claims defines per-consumer token claims that override the provider default.
	// Only valid on the platform-managed LMS token.
	// +kubebuilder:validation:Optional
	Claims *Claims `json:"claims,omitempty"`
}

// ExternalIdentityProvider defines configuration for using an external identity provider
// +kubebuilder:validation:XValidation:rule="self == null || !has(self.basic) || !has(self.client)", message="Only one of basic or client credentials can be provided (XOR relationship)"
type ExternalIdentityProvider struct {
	// TokenEndpoint is the URL for the OAuth2 token endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:MaxLength=2048
	TokenEndpoint string `json:"tokenEndpoint"`

	// TokenRequest configures the token endpoint authentication method (RFC 7591)
	// +kubebuilder:validation:Required
	TokenRequest TokenRequestMethod `json:"tokenRequest"`

	// GrantType defines the OAuth2 grant type to use for the token request
	// +kubebuilder:validation:Required
	GrantType GrantType `json:"grantType"`

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
	// +kubebuilder:validation:MaxLength=256
	Username string `json:"username"`
	// Password for basic authentication
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Password string `json:"password"`
}

// OAuth2ClientCredentials defines client credentials for OAuth2
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.clientKey) ? (!has(self.clientSecret)) : true)", message="ClientSecret and ClientKey cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || has(self.clientSecret) || has(self.clientKey)", message="At least one of clientSecret or clientKey must be provided"
type OAuth2ClientCredentials struct {
	// ClientId identifies the client for OAuth2 client credentials flow
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=256
	ClientId string `json:"clientId"`
	// ClientSecret is the secret associated with the client ID
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=512
	ClientSecret string `json:"clientSecret,omitempty"`
	// ClientKey is the private key associated with the client ID
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxLength=8192
	ClientKey string `json:"clientKey,omitempty"`
}
