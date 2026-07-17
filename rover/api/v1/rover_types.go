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
	// EventExposures are references to EventExposure resources created by this Rover
	EventExposures []types.ObjectRef `json:"eventExposures,omitempty"`
	// EventSubscriptions are references to EventSubscription resources created by this Rover
	EventSubscriptions []types.ObjectRef `json:"eventSubscriptions,omitempty"`
	// FileExposures are references to FileExposure resources created by this Rover in the file domain.
	//
	// TODO(DHEI-20903): today RoverHandler.CreateOrUpdate only initialises this slice
	// (make(..., 0)); it is populated (append of the created file-domain resource refs)
	// by the file handler dispatch once the file domain module is available.
	// Populated from: rover/internal/handler/rover/handler.go, case roverv1.TypeFile.
	FileExposures []types.ObjectRef `json:"fileExposures,omitempty"`
	// FileSubscriptions are references to FileSubscription resources created by this Rover in the file domain.
	//
	// TODO(DHEI-20903): see FileExposures — populated by the file handler dispatch
	// (rover/internal/handler/rover/handler.go, case roverv1.TypeFile) once delivered.
	FileSubscriptions []types.ObjectRef `json:"fileSubscriptions,omitempty"`
	// PermissionSets are references to PermissionSet resources created by this Rover
	PermissionSets []types.ObjectRef `json:"permissionSets,omitempty"`
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

	// Authentication defines the authentication configuration for this application
	// +kubebuilder:validation:Optional
	Authentication *RoverAuthentication `json:"authentication,omitempty"`

	// ClientSecret is the secret used for client authentication
	// If not specified, a randomly generated secret will be used
	// +kubebuilder:validation:Optional
	ClientSecret string `json:"clientSecret"`

	// Exposures is a list of APIs and Events that this Rover exposes to consumers
	// +kubebuilder:validation:Optional
	Exposures []Exposure `json:"exposures,omitempty"`
	// Subscriptions is a list of APIs and Events that this Rover consumes from providers
	// +kubebuilder:validation:Optional
	Subscriptions []Subscription `json:"subscriptions,omitempty"`

	// Permissions defines role-based access control permissions for this application
	// +kubebuilder:validation:Optional
	Permissions []Permission `json:"permissions,omitempty"`

	// ExternalIds carries business identifiers (e.g. PSI, ICTO) attached to this
	// Rover. Each entry is tagged with a scheme. Format and presence are validated
	// per-zone via the zone's ExternalIdPolicies.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=scheme
	// +kubebuilder:validation:MaxItems=16
	ExternalIds []ExternalId `json:"externalIds,omitempty"`
}

// ExternalId is a scheme-tagged business identifier.
type ExternalId struct {
	// Scheme names the identifier system (e.g. "psi", "icto").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9]*$`
	Scheme string `json:"scheme"`

	// Id is the raw identifier value. Per-scheme format rules are applied by the
	// zone's ExternalIdPolicies at admission time.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Id string `json:"id"`
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
	// TypeFile represents a File type resource (SFTP integration)
	TypeFile Type = "file"
)

// FileVariant selects the file-transfer backend used for a file type exposure or
// subscription. Currently only SFTP is supported, as CloudWalker has been retired.
type FileVariant string

// String returns the raw string value of the FileVariant.
//
// TODO(DHEI-20903): currently unused in production code (only asserted in tests).
// It will be called by the file-domain handler
// (rover/internal/handler/rover/file, added in DHEI-20903) when logging/serialising
// the selected variant while creating the file-domain CRD.
func (v FileVariant) String() string {
	return string(v)
}

const (
	// FileVariantSFTP indicates that the file type is handled via the SFTP backend.
	FileVariantSFTP FileVariant = "sftp"
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

// RoverAuthentication defines the top-level authentication configuration for a Rover application
type RoverAuthentication struct {
	// M2M defines machine-to-machine authentication settings for the application
	// +kubebuilder:validation:Optional
	M2M *RoverM2MAuthentication `json:"m2m,omitempty"`
}

// RoverM2MAuthentication defines the M2M authentication settings
type RoverM2MAuthentication struct {
	// TokenRequest configures the token endpoint authentication method (RFC 7591)
	// This feature is currently only documented but not parsed towards the application and identity domain as it is still in discussion whether
	// this should will be enforced for IDPs.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=client_secret_basic
	TokenRequest TokenRequestMethod `json:"tokenRequest,omitempty"`
}

// Exposure defines a service that is exposed by this Rover
// +kubebuilder:validation:XValidation:rule="self == null || has(self.api) || has(self.event) || has(self.file)", message="At least one of api, event or file must be specified"
// +kubebuilder:validation:XValidation:rule="self == null || [has(self.api), has(self.event), has(self.file)].filter(x, x).size() == 1", message="Only one of api, event or file can be specified (XOR relationship)"
type Exposure struct {
	// Api defines an API-based service exposure configuration
	// +kubebuilder:validation:Optional
	Api *ApiExposure `json:"api,omitempty"`
	// Event defines an Event-based service exposure configuration
	// +kubebuilder:validation:Optional
	Event *EventExposure `json:"event,omitempty"`
	// File defines a File-based (SFTP) service exposure configuration
	// +kubebuilder:validation:Optional
	File *FileExposure `json:"file,omitempty"`
}

func (e *Exposure) Type() Type {
	if e.Api != nil {
		return TypeApi
	}
	if e.Event != nil {
		return TypeEvent
	}
	if e.File != nil {
		return TypeFile
	}
	return ""
}

// Subscription defines a service that this Rover consumes
// +kubebuilder:validation:XValidation:rule="self == null || has(self.api) || has(self.event) || has(self.file)", message="At least one of api, event or file must be specified"
// +kubebuilder:validation:XValidation:rule="self == null || [has(self.api), has(self.event), has(self.file)].filter(x, x).size() == 1", message="Only one of api, event or file can be specified (XOR relationship)"
type Subscription struct {
	// Api defines an API-based service subscription configuration
	// +kubebuilder:validation:Optional
	Api *ApiSubscription `json:"api,omitempty"`
	// Event defines an Event-based service subscription configuration
	// +kubebuilder:validation:Optional
	Event *EventSubscription `json:"event,omitempty"`
	// File defines a File-based (SFTP) service subscription configuration
	// +kubebuilder:validation:Optional
	File *FileSubscription `json:"file,omitempty"`
}

func (s *Subscription) Type() Type {
	if s.Api != nil {
		return TypeApi
	}
	if s.Event != nil {
		return TypeEvent
	}
	if s.File != nil {
		return TypeFile
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
	Transformation *Transformation `json:"transformation,omitempty"`
	// Traffic defines optional traffic management configuration for this API
	// +kubebuilder:validation:Optional
	Traffic *Traffic `json:"traffic,omitempty"`
	// Security defines optional security configuration for this API
	// +kubebuilder:validation:Optional
	Security *Security `json:"security,omitempty"`
}

func (apiExp *ApiExposure) HasM2M() bool {
	if apiExp.Security == nil {
		return false
	}

	return apiExp.Security.M2M != nil
}

// EventExposure defines an event that is published by this Rover
type EventExposure struct {
	// EventType identifies the type of event that is published (e.g. "de.telekom.eni.quickstart.v1")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EventType string `json:"eventType"`

	// Visibility defines who can see and subscribe to this event
	// +kubebuilder:validation:Enum=World;Zone;Enterprise
	// +kubebuilder:default=Enterprise
	Visibility Visibility `json:"visibility"`

	// Approval defines the approval workflow required for subscriptions to this event
	// +kubebuilder:validation:Required
	Approval Approval `json:"approval"`

	// Scopes defines named scopes with optional publisher-side trigger filtering
	// +kubebuilder:validation:Optional
	Scopes []EventScope `json:"scopes,omitempty"`

	// AdditionalPublisherIds allows multiple application IDs to publish to the same event type
	// +kubebuilder:validation:Optional
	AdditionalPublisherIds []string `json:"additionalPublisherIds,omitempty"`
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
	// EventType identifies the type of event to subscribe to (e.g. "de.telekom.eni.quickstart.v1")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	EventType string `json:"eventType"`

	// Delivery configures how events are delivered to the subscriber
	// +kubebuilder:validation:Required
	Delivery EventDelivery `json:"delivery"`

	// Trigger defines subscriber-side filtering criteria for event delivery
	// +kubebuilder:validation:Optional
	Trigger *EventTrigger `json:"trigger,omitempty"`

	// Scopes selects which publisher-defined scopes to subscribe to
	// Must match scope names defined on the corresponding EventExposure
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`
}

// FileExposure defines a file type that is exposed by this Rover via SFTP.
// Applying it registers the provider's SSH public keys on the corresponding
// SFTP user (shared space) created from the matching FileSpecification.
type FileExposure struct {
	// FileType identifies the file type that is exposed. It must match the
	// name (and spec.type) of an applied FileSpecification.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	FileType string `json:"fileType"`

	// Variant selects the file-transfer backend. Currently only "sftp" is
	// supported. The field is optional since CloudWalker has been retired.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=sftp
	Variant FileVariant `json:"variant,omitempty"`

	// Visibility defines who can see and subscribe to this file type
	// +kubebuilder:validation:Enum=World;Zone;Enterprise
	// +kubebuilder:default=Enterprise
	Visibility Visibility `json:"visibility"`

	// PublicKeys are the SSH public keys registered for the producer's SFTP user.
	// At least one key is required. Both label and key value must be unique per fileType.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	PublicKeys []PublicKey `json:"publicKeys"`
}

// FileSubscription defines a file type that this Rover consumes via SFTP.
// Applying it registers the consumer's SSH public keys on the corresponding
// SFTP user (shared space) created from the matching FileSpecification.
type FileSubscription struct {
	// FileType identifies the file type to consume. It must match the
	// name of an applied FileSpecification.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	FileType string `json:"fileType"`

	// Variant selects the file-transfer backend. Currently only "sftp" is
	// supported. The field is optional since CloudWalker has been retired.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=sftp
	Variant FileVariant `json:"variant,omitempty"`

	// PublicKeys are the SSH public keys registered for the consumer's SFTP user.
	// At least one key is required. Both label and key value must be unique per fileType.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	PublicKeys []PublicKey `json:"publicKeys"`
}

// PublicKey is a labelled SSH public key registered on a SFTP user.
type PublicKey struct {
	// Label is a human-readable identifier for the key. It must be unique per fileType.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Label string `json:"label"`

	// Key is the SSH public key value. It must be unique per fileType.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
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
