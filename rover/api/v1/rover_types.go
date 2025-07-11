// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoverStatus defines the observed state of Rover
type RoverStatus struct {
	// Conditions represent the latest available observations of the Rover's state
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Application is a reference to the Application resource associated with this Rover
	Application *types.ObjectRef `json:"application,omitempty"`
	// ApiSubscriptions are references to ApiSubscription resources created by this Rover
	ApiSubscriptions []types.ObjectRef `json:"apiSubscriptions,omitempty"`
	// ApiExposures are references to ApiExposure resources created by this Rover
	ApiExposures []types.ObjectRef `json:"apiExposures,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Rover is the Schema for the rovers API
// Rover resources define API exposures and subscriptions for applications
// +kubebuilder:printcolumn:name="Zone",type="string",JSONPath=".spec.zone",description="Zone the Rover belongs to"
type Rover struct {
	// Standard Kubernetes type metadata
	metav1.TypeMeta `json:",inline"`
	// Standard Kubernetes object metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the Rover resource
	// +kubebuilder:validation:Required
	Spec RoverSpec `json:"spec,omitempty"`
	// Status contains the observed state of the Rover resource
	Status RoverStatus `json:"status,omitempty"`
}

var _ types.Object = &Rover{}

func (r *Rover) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Rover) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

//+kubebuilder:object:root=true

// RoverList contains a list of Rover resources
type RoverList struct {
	// Standard Kubernetes type metadata
	metav1.TypeMeta `json:",inline"`
	// Standard Kubernetes list metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is the list of Rover resources
	Items []Rover `json:"items"`
}

var _ types.ObjectList = &RoverList{}

func (r *RoverList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items = append(items, &r.Items[i])
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Rover{}, &RoverList{})
}

// RoverSpec defines the desired state of Rover
type RoverSpec struct {
	// Zone identifies the deployment zone for this Rover resource
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Zone string `json:"zone"`

	// ClientSecret is the secret used for client authentication
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClientSecret string `json:"clientSecret"`

	// Exposures is a list of APIs and Events that this Rover exposes to consumers
	// +kubebuilder:validation:Optional
	Exposures []Exposure `json:"exposures,omitempty"`
	// Subscriptions is a list of APIs and Events that this Rover consumes from providers
	// +kubebuilder:validation:Optional
	Subscriptions []Subscription `json:"subscriptions,omitempty"`
}

// Visibility defines the access scope for an API
type Visibility string

const (
	// VisibilityWorld makes the API accessible to the entire world (public)
	VisibilityWorld Visibility = "World"
	// VisibilityZone makes the API accessible only within the same zone
	VisibilityZone Visibility = "Zone"
	// VisibilityEnterprise makes the API accessible only within the enterprise
	VisibilityEnterprise Visibility = "Enterprise"
)

func (v Visibility) String() string {
	return string(v)
}

// Type defines whether a resource is an API or Event
type Type string

func (t Type) String() string {
	return string(t)
}

const (
	// TypeApi represents an API type resource
	TypeApi Type = "api"
	// TypeEvent represents an Event type resource
	TypeEvent Type = "event"
)

// ApprovalStrategy defines the approval workflow for API exposure
type ApprovalStrategy string

const (
	// ApprovalStrategyAuto provides automatic approval without human intervention
	ApprovalStrategyAuto ApprovalStrategy = "Auto"
	// ApprovalStrategySimple requires approval from one person
	ApprovalStrategySimple ApprovalStrategy = "Simple"
	// ApprovalStrategyFourEyes requires approval from two different people
	ApprovalStrategyFourEyes ApprovalStrategy = "FourEyes"
)

// LoadBalancingStrategy defines how traffic is distributed among multiple upstreams
type LoadBalancingStrategy string

const (
	// LoadBalancingRoundRobin distributes requests evenly across all upstreams
	LoadBalancingRoundRobin LoadBalancingStrategy = "RoundRobin"
	// LoadBalancingLeastConnections sends requests to the upstream with the fewest active connections
	LoadBalancingLeastConnections LoadBalancingStrategy = "LeastConnections"
)

// Exposure defines a service that is exposed by this Rover
// +kubebuilder:validation:XValidation:rule="self == null || has(self.api) || has(self.event)", message="At least one of api or event must be specified"
// +kubebuilder:validation:XValidation:rule="self == null || (!has(self.api) && has(self.event)) || (has(self.api) && !has(self.event))", message="Only one of api or event can be specified (XOR relationship)"
type Exposure struct {
	// Api defines an API-based service exposure configuration
	// +kubebuilder:validation:Optional
	Api *ApiExposure `json:"api,omitempty"`
	// Event defines an Event-based service exposure configuration
	// +kubebuilder:validation:Optional
	Event *EventExposure `json:"event,omitempty"`
}

func (e *Exposure) Type() Type {
	if e.Api != nil {
		return TypeApi
	}
	if e.Event != nil {
		return TypeEvent
	}
	return ""
}

// Subscription defines a service that this Rover consumes
// +kubebuilder:validation:XValidation:rule="self == null || has(self.api) || has(self.event)", message="At least one of api or event must be specified"
// +kubebuilder:validation:XValidation:rule="(has(self.api) && !has(self.event)) || (!has(self.api) && has(self.event))", message="Only one of api or event can be specified (XOR relationship)"
type Subscription struct {
	// Api defines an API-based service subscription configuration
	// +kubebuilder:validation:Optional
	Api *ApiSubscription `json:"api,omitempty"`
	// Event defines an Event-based service subscription configuration
	// +kubebuilder:validation:Optional
	Event *EventSubscription `json:"event,omitempty"`
}

func (s *Subscription) Type() Type {
	if s.Api != nil {
		return TypeApi
	}
	if s.Event != nil {
		return TypeEvent
	}
	return ""
}

// ApiExposure defines an API that is exposed by this Rover
type ApiExposure struct {
	// BasePath is the base path of the API (must start with /)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^/.*$`
	BasePath string `json:"basePath"`
	// Upstreams defines the backend service endpoints for this API
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Upstreams []Upstream `json:"upstreams"`
	// Visibility defines the access scope for this API
	// +kubebuilder:validation:Enum=World;Zone;Enterprise
	// +kubebuilder:default=Enterprise
	Visibility Visibility `json:"visibility"`

	// Approval defines the approval workflow required for this API exposure
	// +kubebuilder:validation:Required
	Approval Approval `json:"approval"`

	// Transformation defines optional request/response transformations for this API
	// +kubebuilder:validation:Optional
	Transformation Transformation `json:"transformation"`
	// Traffic defines optional traffic management configuration for this API
	// +kubebuilder:validation:Optional
	Traffic Traffic `json:"traffic"`
	// Security defines optional security configuration for this API
	// +kubebuilder:validation:Optional
	Security *Security `json:"security,omitempty"`
}

// EventExposure defines an event that is published by this Rover
type EventExposure struct {
	// EventType identifies the type of event that is published
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EventType string `json:"eventType"`
}

// ApiSubscription defines an API that this Rover consumes
type ApiSubscription struct {
	// BasePath is the base path of the API to consume (must start with /)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=^/.*$
	BasePath string `json:"basePath"`
	// Organization is the organization that owns the API
	// Defaults to local organization if not specified
	// +kubebuilder:validation:Optional
	Organization string `json:"organization,omitempty"`

	// Transformation defines optional request/response transformations for this API
	// +kubebuilder:validation:Optional
	Transformation *Transformation `json:"transformation,omitempty"`
	// Traffic defines optional traffic management configuration for this API
	// +kubebuilder:validation:Optional
	Traffic SubscriberTraffic `json:"traffic"`
	// Security defines optional security configuration for this API
	// +kubebuilder:validation:Optional
	Security *SubscriberSecurity `json:"security,omitempty"`
}

func (apiSub *ApiSubscription) HasM2M() bool {
	if apiSub.Security == nil {
		return false
	}

	return apiSub.Security.M2M != nil
}

func (apiSub *ApiSubscription) HasM2MClient() bool {
	if !apiSub.HasM2M() {
		return false
	}

	return apiSub.Security.M2M.Client != nil
}

// EventSubscription defines an event that this Rover subscribes to
type EventSubscription struct {
	// EventType identifies the type of event to subscribe to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EventType string `json:"eventType"`
}

// Approval defines the approval workflow for API exposure
type Approval struct {
	// Strategy defines the approval process required for this API
	// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
	// +kubebuilder:default=Simple
	Strategy ApprovalStrategy `json:"strategy"`
	// TrustedTeams identifies teams that are trusted for approving this API
	// Per default your own team is trusted
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	TrustedTeams []TrustedTeam `json:"trustedTeams,omitempty"`
}

// TrustedTeam identifies a team that is trusted for approvals
type TrustedTeam struct {
	// Group identifies the organizational group for this trusted team
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Group string `json:"group"`
	// Team identifies the specific team within the group
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Team string `json:"team"`
}

// Upstream defines a backend service endpoint for an API
type Upstream struct {
	// URL is the endpoint URL for the backend service
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	URL string `json:"url"`
	// Weight defines the load balancing weight for this upstream (when multiple upstreams)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:validation:Minimum=1
	Weight int `json:"weight,omitempty"`
}

// Transformation defines request/response transformations for an API
// This is shared object for both subscriptions and exposures
type Transformation struct {
	// Request defines transformations applied to incoming API requests
	// +kubebuilder:validation:Optional
	Request RequestResponseTransformation `json:"request"`
}

// RequestResponseTransformation defines transformations applied to API requests and responses
type RequestResponseTransformation struct {
	// Headers defines HTTP header modifications for requests
	// +kubebuilder:validation:Optional
	Headers HeaderTransformation `json:"headers"`
}

// HeaderTransformation defines HTTP header modifications
type HeaderTransformation struct {
	// Remove is a list of HTTP header names to remove
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	Remove []string `json:"remove,omitempty"`
	// Add is a list of HTTP headers to add to the request/response
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	Add []string `json:"add,omitempty"`
}

// Traffic defines traffic management configuration for an API
type Traffic struct {
	// LoadBalancing defines how traffic is distributed among multiple upstream servers
	// +kubebuilder:validation:Optional
	LoadBalancing *LoadBalancing `json:"loadBalancing,omitempty"`
	// Failover defines disaster recovery configuration for this API
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
	// RateLimit defines request rate limiting for this API
	// +kubebuilder:validation:Optional
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

type SubscriberTraffic struct {
	// Failover defines disaster recovery configuration for this API
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
	// RateLimit defines request rate limiting for this API
	// +kubebuilder:validation:Optional
	RateLimit *RateLimitConfig `json:"rateLimit,omitempty"`
}

// LoadBalancing defines load balancing strategy for multiple upstreams
type LoadBalancing struct {
	// Strategy defines the algorithm used for distributing traffic (RoundRobin, LeastConnections)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=RoundRobin;LeastConnections
	// +kubebuilder:default=RoundRobin
	Strategy LoadBalancingStrategy `json:"strategy,omitempty"`
}

// Security defines the security configuration for the Rover
// Security is optional, but if provided, exactly one of m2m or h2m must be set
type Security struct {
	// M2M defines machine-to-machine authentication configuration
	// +kubebuilder:validation:Optional
	M2M *Machine2MachineAuthentication `json:"m2m,omitempty"`
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

// Failover defines failover configuration for disaster recovery
type Failover struct {
	// Zones is a list of zone names to use for failover if the primary zone is unavailable
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=10
	Zones []string `json:"zones,omitempty"`
}

// RateLimit defines rate limiting configuration for an API
type RateLimit struct {
	// Provider defines rate limits applied by the API provider (owner)
	// +kubebuilder:validation:Optional
	Provider *RateLimitConfig `json:"provider,omitempty"`
	// Consumers defines rate limits applied to API consumers (clients)
	// +kubebuilder:validation:Optional
	Consumers *ConsumerRateLimits `json:"consumers,omitempty"`
}

// RateLimitConfig defines rate limits for different time windows
type RateLimitConfig struct {
	// Second defines the maximum number of requests allowed per second
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	Second int `json:"second,omitempty"`
	// Minute defines the maximum number of requests allowed per minute
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	Minute int `json:"minute,omitempty"`
	// Hour defines the maximum number of requests allowed per hour
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	Hour int `json:"hour,omitempty"`
}

// ConsumerRateLimits defines rate limits for API consumers
type ConsumerRateLimits struct {
	// Default defines the rate limit applied to all consumers not specifically overridden
	// +kubebuilder:validation:Optional
	Default *RateLimitConfig `json:"default,omitempty"`
	// Overrides defines consumer-specific rate limits, keyed by consumer identifier
	// +kubebuilder:validation:Optional
	Overrides map[string]RateLimitConfig `json:"overrides,omitempty"`
}

// ExternalIdentityProvider defines configuration for using an external identity provider
// +kubebuilder:validation:XValidation:rule="self == null || (!has(self.basic) && has(self.client)) || (has(self.basic) &&  !has(self.client))", message="Only one of basic or client credentials can be provided (XOR relationship)"
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
	// +kubebuilder:validation:Optional
	ClientId string `json:"clientId"`
	// ClientSecret is the secret associated with the client ID
	// +kubebuilder:validation:Optional
	ClientSecret string `json:"clientSecret"`
}
