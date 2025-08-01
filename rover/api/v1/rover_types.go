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

	// IpRestrictions defines IP-based access restrictions for the entire Application
	// +kubebuilder:validation:Optional
	IpRestrictions *IpRestrictions `json:"ipRestrictions,omitempty"`

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

type IpRestrictions struct {
	// Allow is a list of IP addresses or CIDR ranges that are allowed access
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	// +kubebuilder:validation:Type=array
	// +kubebuilder:validation:XValidation:rule="self.all(x, isCIDR(x) || isIP(x))", message="All items must be valid IP addresses or CIDR notations"
	Allow []string `json:"allow,omitempty"`
	// Deny is a list of IP addresses or CIDR ranges that are denied access
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	// +kubebuilder:validation:Type=array
	// +kubebuilder:validation:XValidation:rule="self.all(x, isCIDR(x) || isIP(x))", message="All items must be valid IP addresses or CIDR notations"
	Deny []string `json:"deny,omitempty"`
}

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
	// +kubebuilder:validation:MaxItems=12
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
	Transformation *Transformation `json:"transformation"`
	// Traffic defines optional traffic management configuration for this API
	// +kubebuilder:validation:Optional
	Traffic *Traffic `json:"traffic"`
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
