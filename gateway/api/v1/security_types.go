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

type Security struct {
	// DisableAccessControl disable the ACL mechanism for this route
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	DisableAccessControl bool `json:"disableAccessControl,omitempty"`

	// DefaultConsumers defines a list of default consumers that are allowed to access this route without being explicitly added as a consumer
	// +kubebuilder:validation:Optional
	DefaultConsumers []string `json:"defaultConsumers,omitempty"`

	// TrustedIssuers defines a list of trusted token issuers for this route. If empty, all issuers are trusted.
	// +kubebuilder:validation:Optional
	// +listType=set
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:items:Format=uri
	TrustedIssuers []string `json:"trustedIssuers,omitempty"`

	// RealmName defines the realm name for this route, which is used in the Jumper sidecar to determine the Last-Mile-Token
	// +kubebuilder:validation:Required
	RealmName string `json:"realmName"`

	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *Machine2MachineAuthentication `json:"m2m,omitempty"`
}

// ClaimValueFrom is a source Jumper resolves at runtime into the claim value.
// +kubebuilder:validation:Enum=ConsumerClientId
type ClaimValueFrom string

const (
	ClaimValueFromConsumerClientId ClaimValueFrom = "ConsumerClientId"
)

// Claim is a single token claim written into JumperConfig.Claims.
// Value is a CP-resolved literal; ValueFrom is resolved by Jumper at runtime.
// Exactly one of value or valueFrom must be set.
// +kubebuilder:validation:XValidation:rule="has(self.value) != has(self.valueFrom)",message="exactly one of value or valueFrom must be set"
type Claim struct {
	// Key is the claim name (e.g. "aud")
	// +kubebuilder:validation:Required
	Key string `json:"key"`
	// Value is the CP-resolved literal claim value
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value,omitempty"`
	// ValueFrom is a runtime source Jumper resolves (e.g. ConsumerClientId)
	// +kubebuilder:validation:Optional
	ValueFrom ClaimValueFrom `json:"valueFrom,omitempty"`
}

func (s *Security) HasM2M() bool {
	return s.M2M != nil
}

func (s *Security) HasM2MExternalIDP() bool {
	if !s.HasM2M() {
		return false
	}
	return s.M2M.ExternalIDP != nil
}

func (s *Security) HasBasicAuth() bool {
	if !s.HasM2M() {
		return false
	}
	return s.M2M.Basic != nil
}

// Security defines the security configuration for the Rover
// Security is optional, but if provided, exactly one of m2m or h2m must be set
type ConsumerSecurity struct {
	IpRestrictions *IpRestrictions `json:"ipRestrictions,omitempty"`
}

type IpRestrictions struct {
	// +kubebuilder:validation:Optional
	Allow []string `json:"allow,omitempty"`
	// +kubebuilder:validation:Optional
	Deny []string `json:"deny,omitempty"`
}

type ConsumeRouteSecurity struct {
	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *ConsumerMachine2MachineAuthentication `json:"m2m,omitempty"`
}

func (s *ConsumeRouteSecurity) HasM2M() bool {
	return s.M2M != nil
}

func (s *ConsumeRouteSecurity) HasBasicAuth() bool {
	if !s.HasM2M() {
		return false
	}
	return s.M2M.Basic != nil
}

// Machine2MachineAuthentication defines the authentication methods for machine-to-machine communication
// Either externalIDP, basic, or only scopes can be provided
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.externalIDP) ? (!has(self.basic)) : true)", message="ExternalIDP and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.scopes) ? (!has(self.basic)) : true)", message="Scopes and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || has(self.externalIDP) || has(self.basic) || has(self.scopes) || has(self.claims)", message="At least one of externalIDP, basic, scopes, or claims must be provided"
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
	// Claims defines token claims applied to all consumers (the "default" bucket)
	// +kubebuilder:validation:Optional
	Claims []Claim `json:"claims,omitempty"`
}

// ConsumerMachine2MachineAuthentication defines the authentication methods for machine-to-machine communication for consumers
// Either client, basic, or only scopes can be provided
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.client) ? (!has(self.basic)) : true)", message="Client and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.scopes) ? (!has(self.basic)) : true)", message="Scopes and basic authentication cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || has(self.client) || has(self.basic) || has(self.scopes) || has(self.claims)", message="At least one of client, basic, scopes, or claims must be provided"
type ConsumerMachine2MachineAuthentication struct {
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
	// Claims defines per-consumer token claims (override the default bucket for this consumer)
	// +kubebuilder:validation:Optional
	Claims []Claim `json:"claims,omitempty"`
}

// ExternalIdentityProvider defines configuration for using an external identity provider
// +kubebuilder:validation:XValidation:rule="self == null || !has(self.basic) || !has(self.client)", message="Only one of basic or client credentials can be provided (XOR relationship)"
type ExternalIdentityProvider struct {
	// TokenEndpoint is the URL for the OAuth2 token endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	TokenEndpoint string `json:"tokenEndpoint"`

	// TokenRequest configures the token endpoint authentication method (RFC 7591)
	// +kubebuilder:validation:Required
	TokenRequest TokenRequestMethod `json:"tokenRequest"`
	// GrantType is the grant type for the external IDP authentication
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
	Username string `json:"username"`
	// Password for basic authentication
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Password string `json:"password"`
}

// OAuth2ClientCredentials defines client credentials for OAuth2
// Either clientSecret or clientKey can be provided, but not both
// +kubebuilder:validation:XValidation:rule="self == null || (has(self.clientKey) ? (!has(self.clientSecret)) : true)", message="ClientSecret and ClientKey cannot be used together"
// +kubebuilder:validation:XValidation:rule="self == null || has(self.clientSecret) || has(self.clientKey)", message="At least one of clientSecret or clientKey must be provided"
type OAuth2ClientCredentials struct {
	// ClientId identifies the client for OAuth2 client credentials flow
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientId string `json:"clientId"`
	// ClientSecret is the secret associated with the client ID
	// +kubebuilder:validation:Optional
	ClientSecret string `json:"clientSecret,omitempty"`
	// clientKey is the private key associated with the client ID
	// +kubebuilder:validation:Optional
	ClientKey string `json:"clientKey,omitempty"`
}
