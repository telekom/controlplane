// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/telekom/controlplane/common/pkg/reminder"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ZoneVisibility string

const (
	ZoneVisibilityWorld      ZoneVisibility = "World"
	ZoneVisibilityEnterprise ZoneVisibility = "Enterprise"
)

var (
	ErrNoPresetFound = fmt.Errorf("no preset found with the specified name")
)

type RedisConfig struct {
	// Host is the Redis server hostname (e.g. "redis-master.svc.cluster.local").
	// +kubebuilder:validation:Required
	Host string `json:"host"`
	// Port is the Redis server port.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=6379
	Port int `json:"port,omitempty"`
	// Password is a reference to the Redis password in the secret manager.
	// +kubebuilder:validation:Optional
	Password string `json:"password,omitempty"`
	// EnableTLS controls whether TLS is used for the Redis connection.
	// +kubebuilder:validation:Optional
	EnableTLS bool `json:"enableTLS,omitempty"`
}

type IdentityProviderAdminConfig struct {
	// Url is the base URL of the identity provider admin API.
	// If empty, the operator will attempt to discover the URL based on the provided IdentityProvider Url.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=uri
	Url *string `json:"url,omitempty"`
	// ClientId is the client ID to authenticate with the identity provider admin API.
	ClientId string `json:"clientId"`
	// UserName is the username to authenticate with the identity provider admin API.
	UserName string `json:"userName"`
	// Password is the password to authenticate with the identity provider admin API.
	Password string `json:"password"`
}

type SecretRotationConfig struct {
	// Enabled controls whether secret rotation is enabled for this zone.
	//  If false, secrets will not be rotated and the grace period and expiration period will be ignored.
	Enabled bool `json:"enabled"`
	// GracePeriod is the duration that a rotated secret is valid for.
	// This allows to have a smooth transition when rotating secrets, as the old secret is still valid for a certain period of time after rotation.
	// +kubebuilder:validation:Required
	GracePeriod metav1.Duration `json:"gracePeriod"`

	// ExpirationPeriod is the duration that the current secret is valid for.
	// Once this period has elapsed, the secret is considered expired and should be rotated.
	// +kubebuilder:validation:Required
	ExpirationPeriod metav1.Duration `json:"expirationPeriod"`

	// NotificationThresholds defines the schedule of reminder notifications before
	// secret expiry. Each entry triggers a notification when the remaining time-to-expiry
	// crosses that threshold. Only the tightest (smallest) matching threshold is evaluated
	// per reconciliation cycle to avoid spamming.
	//
	// Example: [{before: "720h"}, {before: "168h", repeat: "24h"}]
	// → single reminder at 30 days, then daily reminders starting at 7 days.
	// +kubebuilder:validation:MinItems=1
	NotificationThresholds []reminder.Threshold `json:"notificationThresholds"`
}

type IdentityProviderConfig struct {
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(-?[a-z0-9]+)*$`
	Name  string                      `json:"name"`
	Admin IdentityProviderAdminConfig `json:"admin"`
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Format=hostname
	IssuerHostname string `json:"issuerHostname,omitempty"`
	// +kubebuilder:validation:Format=uri
	TokenUrl       string                `json:"tokenUrl,omitempty"`
	SecretRotation *SecretRotationConfig `json:"secretRotation,omitempty"`
}

// GatewayAdminConfig contains the necessary information to connect to the gateway admin API for this zone.
// Most of it can be optional if the Gateway was setup to support it, then only the URL is required.
type GatewayAdminConfig struct {
	// IdentityProviderRef selects the IDP used to obtain gateway-admin tokens.
	IdentityProviderRef string `json:"identityProviderRef"`

	// URL of the gateway admin API.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	Url string `json:"url"`

	// ClientId of the admin client.
	// If empty, a managed client with the default name will be used.
	// +kubebuilder:validation:Optional
	ClientId *string `json:"clientId,omitempty"`
	// ClientSecret of the admin client
	// If empty, a managed client secret will be generated.
	// +kubebuilder:validation:Optional
	ClientSecret *string `json:"clientSecret,omitempty"`
}

// UrlConfig defines the configuration for a single URL (hostname + base path) exposed by the gateway for this zone.
type UrlConfig struct {
	// Hostname is the hostname part of the URL (e.g. "api.example.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Format=hostname
	Hostname string `json:"hostname"`
	// Port is the port number of the URL (e.g. 8000). If not set, the default port for the scheme is used (443 for https, 80 for http).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
	// Scheme is the URL scheme (e.g. "http" or "https"). Defaults to "https" if not set.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=http;https
	// +kubebuilder:default=https
	Scheme string `json:"scheme,omitempty"`
	// BasePath is the base path part of the URL which will be the prefix of all routes exposed on this URL (e.g. "/v1").
	// It is appended to the hostname to construct the full URL (e.g. "https://api.example.com/v1").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^/.*$`
	BasePath string `json:"basePath"`
	// Hidden controls whether this URL should be hidden from the Links section in the Zone status.
	// This can be used to hide internal-only URLs that should not be exposed to API consumers.
	Hidden bool `json:"hidden"`
}

func (u UrlConfig) GetScheme() string {
	if u.Scheme == "" {
		return "https"
	}
	return u.Scheme
}

func (u UrlConfig) GetFullUrl() string {
	scheme := u.GetScheme()
	if u.Port != 0 {
		return fmt.Sprintf("%s://%s:%d%s", scheme, u.Hostname, u.Port, u.BasePath)
	}
	return fmt.Sprintf("%s://%s%s", scheme, u.Hostname, u.BasePath)
}

type Preset struct {
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(-?[a-z0-9]+)*$`
	Name                string `json:"name"`
	Default             bool   `json:"default,omitempty"`
	GatewayRef          string `json:"gatewayRef"`
	IdentityProviderRef string `json:"identityProviderRef"`
	// +kubebuilder:validation:Format=uri
	TokenUrl string `json:"tokenUrl,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	Urls []UrlConfig `json:"urls"`
	// +listType=map
	// +listMapKey=name
	Features []Feature `json:"features,omitempty"`
}

// ResolveHostnamesAndPaths derives route hostnames and paths from the preset's URL configuration.
// Each URL contributes one hostname and one path (basePath + routePath).
func (p *Preset) ResolveHostnamesAndPaths(routePath string) (hostnames []string, paths []string) {
	for _, u := range p.Urls {
		hostnames = append(hostnames, u.Hostname)
		paths = append(paths, path.Join(u.BasePath, routePath))
	}
	return
}

// GetDefaultURL returns the full URL of the first non-hidden UrlConfig in this preset, or an empty string if all UrlConfigs are hidden.
func (p *Preset) GetDefaultURL() string {
	for _, u := range p.Urls {
		if !u.Hidden {
			return u.GetFullUrl()
		}
	}
	return ""
}

func (p *Preset) SupportsFeatures(featureNames []FeatureName) bool {
	return supportsFeatures(p.Features, featureNames...)
}

type GatewayConfig struct {
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(-?[a-z0-9]+)*$`
	Name  string             `json:"name"`
	Admin GatewayAdminConfig `json:"admin"`
}

// ManagedRouteType defines the type of a managed route.
// +kubebuilder:validation:Enum=TeamAPI;Proxy
type ManagedRouteType string

const (
	// ManagedRouteTypeTeamAPI creates a route with authentication (PassThrough=false)
	// and disabled access control on the zone's team-api gateway realm.
	// Used for team APIs that require token validation but no per-consumer ACLs.
	ManagedRouteTypeTeamAPI ManagedRouteType = "TeamAPI"

	// ManagedRouteTypeProxy creates a fully passthrough route (PassThrough=true)
	// on the zone's default gateway realm that acts as a pure reverse proxy
	// without any authentication or authorization.
	ManagedRouteTypeProxy ManagedRouteType = "Proxy"
)

type ManagedRouteConfig struct {
	// Name is the name of the created route. It must be unique within the zone.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=^[a-z0-9]+(-?[a-z0-9]+)*$
	Name string `json:"name"`

	// Path is the path of the route exposed on the gateway.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^/.*$`
	Path string `json:"path"`
	// Url is the upstream URL of the route.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	Url string `json:"url"`
	// Type selects the route behavior: TeamAPI (authenticated, no ACL) or Proxy (passthrough reverse proxy).
	// +kubebuilder:validation:Required
	Type ManagedRouteType `json:"type"`
}

// ManagedRoutesConfig defines the configuration for managed routes in a zone.
// Managed routes are automatically created and managed by the system based on this configuration.
type ManagedRoutesConfig struct {
	// Routes is the list of routes to be created for this zone.
	// It may be used to create additional routes that are required for operating the zone
	// +optional
	Routes []ManagedRouteConfig `json:"routes"`
}

type PermissionsConfig struct {
	// ApiBasePath is the base path for the permission service API endpoint
	// Format: /eni/chevron/v2/permission
	// This will be appended to the gateway URL to build the full permissions URL
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^/.*`
	ApiBasePath string `json:"apiBasePath"`

	// ConsoleUrl is the admin UI for the permission service (optional)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=uri
	ConsoleUrl *string `json:"consoleUrl,omitempty"`
}

// ExternalIdPolicy configures validation for a single external identifier scheme
// on Rovers and Applications in this zone.
type ExternalIdPolicy struct {
	// Scheme names the identifier system (e.g. "psi", "icto").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9]*$`
	Scheme string `json:"scheme"`

	// Required acts as a per-zone feature flag controlling whether this scheme
	// is mandatory in this zone. When true, every Rover/Application in this
	// zone MUST carry an externalIds entry with this scheme. The id's format
	// is always checked against Pattern whenever an entry with this scheme is
	// supplied, regardless of Required.
	// +kubebuilder:default=false
	Required bool `json:"required"`

	// Pattern is the ECMA 262 regex the id must match. Always enforced when an
	// externalIds entry with this scheme is present; also drives the
	// presence-check error when Required is true.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Pattern string `json:"pattern"`
}

// ZoneSpec defines the desired state of Zone
type ZoneSpec struct {
	// Features configures capabilities that apply to the entire zone.
	// +listType=map
	// +listMapKey=name
	Features []Feature `json:"features,omitempty"`
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	Gateways []GatewayConfig `json:"gateways"`
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	IdentityProviders []IdentityProviderConfig `json:"identityProviders"`
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	Presets       []Preset             `json:"presets"`
	Redis         *RedisConfig         `json:"redis,omitempty"`
	ManagedRoutes *ManagedRoutesConfig `json:"managedRoutes,omitempty"`
	// +kubebuilder:validation:Enum=World;Enterprise
	// Visibility controls what subscriptions are allowed from and to this zone. It's also relevant for features like failover
	Visibility ZoneVisibility `json:"visibility"`

	// Permissions configuration for permission service integration
	// +kubebuilder:validation:Optional
	Permissions *PermissionsConfig `json:"permissions,omitempty"`

	// ExternalIdPolicies configures, per identifier scheme, the format and
	// presence requirements for externalIds on Rovers and Applications bound to
	// this zone. Empty means no enforcement for any scheme.
	// +kubebuilder:validation:Optional
	// +listType=map
	// +listMapKey=scheme
	// +kubebuilder:validation:MaxItems=16
	ExternalIdPolicies []ExternalIdPolicy `json:"externalIdPolicies,omitempty"`
}

func (s *ZoneSpec) GetGateway(name string) (*GatewayConfig, error) {
	for i := range s.Gateways {
		if s.Gateways[i].Name == name {
			return &s.Gateways[i], nil
		}
	}
	return nil, fmt.Errorf("gateway %q not found", name)
}

func (s *ZoneSpec) GetIdentityProvider() (*IdentityProviderConfig, error) {
	if len(s.IdentityProviders) != 1 {
		return nil, fmt.Errorf("exactly one identity provider is required, got %d", len(s.IdentityProviders))
	}
	return &s.IdentityProviders[0], nil
}

func (s *ZoneSpec) GetIdentityProviderByName(name string) (*IdentityProviderConfig, error) {
	for i := range s.IdentityProviders {
		if s.IdentityProviders[i].Name == name {
			return &s.IdentityProviders[i], nil
		}
	}
	return nil, fmt.Errorf("identity provider %q not found", name)
}

func (s *ZoneSpec) GetPreset(name string) (*Preset, error) {
	for i := range s.Presets {
		if s.Presets[i].Name == name {
			return &s.Presets[i], nil
		}
	}
	return nil, fmt.Errorf("preset %q: %w", name, ErrNoPresetFound)
}

func (s *ZoneSpec) GetDefaultPreset() (*Preset, error) {
	var match *Preset
	for i := range s.Presets {
		if !s.Presets[i].Default {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("multiple default presets")
		}
		match = &s.Presets[i]
	}
	if match == nil {
		return nil, fmt.Errorf("default preset: %w", ErrNoPresetFound)
	}
	return match, nil
}

func (s *ZoneSpec) SelectPreset(features ...FeatureName) (*Preset, error) {
	if len(features) == 0 {
		return nil, fmt.Errorf("no features requested")
	}
	presetFeatures := make([]FeatureName, 0, len(features))
	for _, feature := range features {
		switch FeatureScopeOf(feature) {
		case FeatureScopeZone:
			if !supportsFeatures(s.Features, feature) {
				return nil, fmt.Errorf("zone feature %q is not enabled", feature)
			}
		case FeatureScopePreset:
			presetFeatures = append(presetFeatures, feature)
		default:
			return nil, fmt.Errorf("unknown feature %q", feature)
		}
	}
	if len(presetFeatures) == 0 {
		return s.GetDefaultPreset()
	}
	var match *Preset
	for i := range s.Presets {
		if !s.Presets[i].SupportsFeatures(presetFeatures) {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("multiple presets match features %v", presetFeatures)
		}
		match = &s.Presets[i]
	}
	if match == nil {
		return nil, fmt.Errorf("no preset matches features %v", presetFeatures)
	}
	return match, nil
}

func (s *ZoneSpec) FeaturesSupported(features ...FeatureName) bool {
	_, err := s.SelectPreset(features...)
	return err == nil
}

type Links struct {
	// Url is the base URL of the default gateway of this zone
	// +kubebuilder:validation:Format=uri
	Url string `json:"gatewayUrl"`
	// Issuer is the expected issuer of downstream tokens for this zone
	// +kubebuilder:validation:Format=uri
	Issuer string `json:"gatewayIssuer"`
	// TeamIssuer is the expected issuer of downstream tokens for Team APIs in this zone
	// +kubebuilder:validation:Format=uri
	// +optional
	TeamIssuer string `json:"teamApiIssuer,omitempty"`
	// LmsIssuer is the issuer of the Last-Mile-Security tokens (upstream) for this zone
	// +kubebuilder:validation:Format=uri
	// +optional
	LmsIssuer string `json:"gatewayLmsIssuer"`
	// InternalIssuer is the expected issuer of downstream tokens for internal services in this zone
	// +kubebuilder:validation:Format=uri
	// +optional
	InternalIssuer string `json:"internalIssuer,omitempty"`

	// PermissionsUrl for permission queries (dynamically built from gateway URL)
	// Format: https://<gateway>/eni/chevron/v2/permission
	// Applications append ?application=<clientId> when querying
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=uri
	PermissionsUrl string `json:"permissionsUrl,omitempty"`
}

type GatewayStatus struct {
	Name        string           `json:"name"`
	Gateway     *types.ObjectRef `json:"gateway,omitempty"`
	AdminClient *types.ObjectRef `json:"adminClient,omitempty"`
	Consumer    *types.ObjectRef `json:"consumer,omitempty"`
}

type PresetStatus struct {
	Name                string           `json:"name"`
	GatewayRef          *types.ObjectRef `json:"gatewayRef,omitempty"`
	IdentityProviderRef *types.ObjectRef `json:"identityProviderRef,omitempty"`
	Links               Links            `json:"links,omitempty"`
	TokenUrl            string           `json:"tokenUrl,omitempty"`
}

// ZoneStatus defines the observed state of Zone
type ZoneStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	Namespace             string           `json:"namespace,omitempty"`
	IdentityProvider      *types.ObjectRef `json:"identityProvider,omitempty"`
	IdentityRealm         *types.ObjectRef `json:"identityRealm,omitempty"`
	InternalIdentityRealm *types.ObjectRef `json:"internalIdentityRealm,omitempty"`

	// +listType=map
	// +listMapKey=name
	Gateways []GatewayStatus `json:"gateways,omitempty"`
	// +listType=map
	// +listMapKey=name
	Presets []PresetStatus `json:"presets,omitempty"`

	TeamApiIdentityRealm *types.ObjectRef  `json:"teamApiIdentityRealm,omitempty"`
	ManagedRoutes        []types.ObjectRef `json:"managedRoutes,omitempty"`

	// RealmName as an abstraction layer and is retrieved from Env.Spec.RealmName
	RealmName string `json:"realmName,omitempty"`

	// Features is a list of features that are enabled or disabled for this zone.
	// This can be used to control the availability of certain features in the zone
	// +listType=map
	// +listMapKey=name
	// +patchStrategy=merge
	// +patchMergeKey=name
	// +optional
	Features []Feature `json:"features,omitempty"`
}

func (s *ZoneStatus) GetGateway(name string) (*GatewayStatus, error) {
	for i := range s.Gateways {
		if s.Gateways[i].Name == name {
			return &s.Gateways[i], nil
		}
	}
	return nil, fmt.Errorf("gateway status %q not found", name)
}

func (s *ZoneStatus) GetPreset(name string) (*PresetStatus, error) {
	for i := range s.Presets {
		if s.Presets[i].Name == name {
			return &s.Presets[i], nil
		}
	}
	return nil, fmt.Errorf("preset status %q not found", name)
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Zone is the Schema for the zones API
// Group is the Schema for the groups API.
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]+(-?[a-z0-9]+)*$')",message="metadata.name must match the pattern ^[a-z0-9]+(-?[a-z0-9]+)*$"
type Zone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZoneSpec   `json:"spec,omitempty"`
	Status ZoneStatus `json:"status,omitempty"`
}

var _ types.Object = &Zone{}

func (z *Zone) GetConditions() []metav1.Condition {
	return z.Status.Conditions
}

func (z *Zone) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&z.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ZoneList contains a list of Zone
type ZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Zone `json:"items"`
}

var _ types.ObjectList = &ZoneList{}

func (l *ZoneList) GetItems() []types.Object {
	items := make([]types.Object, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Zone{}, &ZoneList{})
}

// Feature Management

type FeatureName string

const (
	// FeatureBasicAuth indicates that Basic Authentication is available throughout the zone.
	FeatureBasicAuth FeatureName = "BasicAuth"

	// FeatureSecretRotation is a reconciled status feature indicating secret rotation is enabled.
	FeatureSecretRotation FeatureName = "SecretRotation"

	// FeatureAiGateway indicates that the AI Gateway is configured and available for this zone.
	FeatureAiGateway FeatureName = "AiGateway"

	// FeaturePermissions is a reconciled status feature indicating permission service integration is enabled.
	FeaturePermissions FeatureName = "Permissions"

	// FeatureConsumerFailover indicates that consumer failover is enabled for the zone.
	// This feature is automatically enabled if the Zone has a "ConsumerFailover" gateway preset configured.
	FeatureConsumerFailover FeatureName = "ConsumerFailover"

	// FeatureRateLimiting is a reconciled status feature indicating rate limiting is enabled.
	// The zone requires a valid Redis configuration to support rate limiting
	FeatureRateLimiting FeatureName = "RateLimiting"
)

type FeatureScope string

const (
	FeatureScopeUnknown FeatureScope = ""
	FeatureScopeZone    FeatureScope = "Zone"
	FeatureScopePreset  FeatureScope = "Preset"
)

func FeatureScopeOf(feature FeatureName) FeatureScope {
	switch feature {
	case FeatureBasicAuth:
		return FeatureScopeZone
	case FeatureAiGateway, FeatureConsumerFailover:
		return FeatureScopePreset
	default:
		return FeatureScopeUnknown
	}
}

type Feature struct {
	Name    FeatureName `json:"name"`
	Enabled bool        `json:"enabled"`
}

func supportsFeatures(configured []Feature, requested ...FeatureName) bool {
	for _, featureName := range requested {
		if !slices.ContainsFunc(configured, func(feature Feature) bool {
			return strings.EqualFold(string(feature.Name), string(featureName)) && feature.Enabled
		}) {
			return false
		}
	}
	return true
}

func (z *Zone) IsFeatureEnabled(featureName FeatureName) bool {
	for _, feature := range z.Status.Features {
		if strings.EqualFold(string(featureName), string(feature.Name)) {
			return feature.Enabled
		}
	}
	return false
}

func (z *Zone) EnableFeature(featureName FeatureName) {
	z.ManageFeature(featureName, true)
}

func (z *Zone) ManageFeature(featureName FeatureName, enabled bool) {
	for i, feature := range z.Status.Features {
		if feature.Name == featureName {
			z.Status.Features[i].Enabled = enabled
			return
		}
	}
	z.Status.Features = append(z.Status.Features, Feature{Name: featureName, Enabled: enabled})
}
