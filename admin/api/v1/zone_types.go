// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/reminder"
	"github.com/telekom/controlplane/common/pkg/types"
)

// Preset resolution sentinels. Downstream callers branch on these, so the distinction
// between "the zone offers nothing" and "the zone is broken" must stay sharp.
var (
	// ErrNoMatchingPreset means the request was well-formed but this zone offers no preset
	// providing it. The zone is valid, so callers may treat it as "skip this zone". It must
	// never wrap a malformed request or a misconfigured zone.
	ErrNoMatchingPreset = errors.New("no matching preset")

	// ErrInvalidFeatures means the request itself is malformed: a feature is unknown, or is
	// zone-scoped and not enabled. Callers must not skip on it — the caller, not the zone,
	// is at fault.
	ErrInvalidFeatures = errors.New("invalid features")

	// ErrAmbiguousPreset means the zone is misconfigured: several presets of one traffic type
	// are marked default, or several carry the same enabled feature, so selection cannot be
	// single-valued. Callers must not skip on it; surface it as a blocking error.
	ErrAmbiguousPreset = errors.New("ambiguous preset")

	// ErrNoDefaultPreset means presets of the requested traffic type exist but none is marked
	// default. The zone is misconfigured, so callers must not skip on it.
	ErrNoDefaultPreset = errors.New("no default preset")

	// ErrNoPresetFound means no preset exists with the requested name. It belongs to lookup by
	// name, not to selection.
	ErrNoPresetFound = errors.New("no preset found with the specified name")
)

type ZoneVisibility string

const (
	ZoneVisibilityWorld      ZoneVisibility = "World"
	ZoneVisibilityEnterprise ZoneVisibility = "Enterprise"
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
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^/.*$`
	// +kubebuilder:default=/
	BasePath string `json:"basePath"`
	// Hidden controls whether this URL should be hidden from the Links section in the Zone status.
	// This can be used to hide internal-only URLs that should not be exposed to API consumers.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
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
	// Name is the unique name of the preset within the zone.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(-?[a-z0-9]+)*$`
	Name string `json:"name"`
	// Type is the kind of traffic this preset routes. It selects which domain's
	// routes use this preset; a gateway serves the union of its presets' types.
	// +kubebuilder:validation:Required
	Type GatewayType `json:"type"`
	// +kubebuilder:validation:Optional
	Default bool `json:"default,omitempty"`
	// GatewayRef is the name of the gateway to used for this preset.
	// It must match one of the gateways defined in the zone spec.
	// +kubebuilder:validation:Required
	GatewayRef string `json:"gatewayRef"`
	// IdentityProviderRef is the name of the identity provider to used for this preset.
	// It must match one of the identity providers defined in the zone spec.
	// +kubebuilder:validation:Required
	IdentityProviderRef string `json:"identityProviderRef"`
	// TokenUrl is the token endpoint for this zone preset
	// By default it is derived from the identity provider's issuer URL, but can be overridden here if needed.
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:Optional
	TokenUrl string `json:"tokenUrl,omitempty"`
	// Urls is the list of URLs (hostnames + base paths) exposed by the gateway for this preset.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	Urls []UrlConfig `json:"urls"`
	// Features is a list of features that are enabled or disabled for this preset.
	// +listType=map
	// +listMapKey=name
	Features []Feature `json:"features,omitempty"`
}

// ResolveHostnamesAndPaths derives route hostnames and paths from the preset's URL configuration.
// Each URL contributes one hostname and one path (basePath + routePath).
func (p *Preset) ResolveHostnamesAndPaths(routePath string) (hostnames, paths []string) {
	for _, u := range p.Urls {
		hostnames = append(hostnames, u.Hostname)
		paths = append(paths, path.Join(u.BasePath, routePath))
	}
	return hostnames, paths
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

// GatewayType classifies the kind of traffic a gateway serves.
// +kubebuilder:validation:Enum=API;AI;Event
type GatewayType string

const (
	// GatewayTypeAPI serves synchronous API traffic (exposures, subscriptions, proxy routes).
	GatewayTypeAPI GatewayType = "API"
	// GatewayTypeAI serves agentic traffic (MCP servers, agent cards).
	GatewayTypeAI GatewayType = "AI"
	// GatewayTypeEvent serves asynchronous event traffic.
	GatewayTypeEvent GatewayType = "Event"
)

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
	// +kubebuilder:validation:MaxItems=10
	Gateways []GatewayConfig `json:"gateways"`
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	IdentityProviders []IdentityProviderConfig `json:"identityProviders"`
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
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

// GetDefaultPreset returns the zone's representative profile: the API type default.
// Callers that carry no traffic type (projector, team routes, team consumers) use this
// rather than inventing one. It is equivalent to SelectPreset(GatewayTypeAPI).
func (s *ZoneSpec) GetDefaultPreset() (*Preset, error) {
	return s.SelectPreset(GatewayTypeAPI)
}

func (s *ZoneSpec) validateFeatures(features []FeatureName) ([]FeatureName, []string) {
	var problems []string
	presetFeatures := make([]FeatureName, 0, len(features))
	for _, feature := range features {
		switch FeatureScopeOf(feature) {
		case FeatureScopeZone:
			if !supportsFeatures(s.Features, feature) {
				problems = append(problems, fmt.Sprintf("zone feature %q is not enabled", feature))
			}
		case FeatureScopePreset:
			presetFeatures = append(presetFeatures, feature)
		default:
			problems = append(problems, fmt.Sprintf("feature %q is unknown", feature))
		}
	}
	return presetFeatures, problems
}

// presetsOfType returns the presets serving one traffic type, in spec order.
func (s *ZoneSpec) presetsOfType(gatewayType GatewayType) []*Preset {
	var presets []*Preset
	for i := range s.Presets {
		if s.Presets[i].Type == gatewayType {
			presets = append(presets, &s.Presets[i])
		}
	}
	return presets
}

// explainMissingFeatures reports which requested features no preset of the type enables.
func (s *ZoneSpec) explainMissingFeatures(gatewayType GatewayType, presetFeatures []FeatureName) []string {
	candidates := s.presetsOfType(gatewayType)
	var problems []string
	for _, feature := range presetFeatures {
		if !slices.ContainsFunc(candidates, func(p *Preset) bool { return supportsFeatures(p.Features, feature) }) {
			problems = append(problems, fmt.Sprintf("feature %q is not enabled on any %q preset", feature, gatewayType))
		}
	}
	return problems
}

// selectionScope names what was asked for, for the aggregate selection errors. The feature
// clause is dropped when no features were requested: "feature combination []" is pure noise,
// and the gateway type already appears in the problems it is joined with.
func selectionScope(gatewayType GatewayType, features []FeatureName) string {
	if len(features) == 0 {
		return fmt.Sprintf("gateway type %q", gatewayType)
	}
	return fmt.Sprintf("feature combination %v for gateway type %q", features, gatewayType)
}

// SelectPreset returns the single routing profile for a traffic type and feature set.
// The webhook is expected to guarantee one feature-free default per type and at most one
// preset per enabled preset-scoped feature per type; both invariants are re-checked here
// rather than silently resolved by spec order, because a violation is a misconfiguration.
func (s *ZoneSpec) SelectPreset(gatewayType GatewayType, features ...FeatureName) (*Preset, error) {
	presetFeatures, problems := s.validateFeatures(features)
	// A malformed request and an unavailable capability are different classes: callers use the
	// sentinel to tell "this zone does not offer it, skip" from "this is misconfigured, block".
	sentinel := ErrNoMatchingPreset
	if len(problems) > 0 {
		sentinel = ErrInvalidFeatures
	}

	candidates := s.presetsOfType(gatewayType)
	if len(candidates) == 0 {
		problems = append(problems, fmt.Sprintf("no preset of type %q exists", gatewayType))
		return nil, fmt.Errorf("%w: %s: %s",
			sentinel, selectionScope(gatewayType, features), strings.Join(problems, "; "))
	}

	var matches []*Preset
	for _, preset := range candidates {
		if preset.SupportsFeatures(presetFeatures) {
			matches = append(matches, preset)
		}
	}

	if len(matches) == 0 {
		missing := s.explainMissingFeatures(gatewayType, presetFeatures)
		problems = append(problems, missing...)
		if len(missing) == 0 && len(presetFeatures) > 1 {
			// ponytail: unreachable while ConsumerFailover is the only preset-scoped feature,
			// but required so we never return (nil, nil) once a second one exists.
			problems = append(problems, "requested features are enabled on different presets, but only one preset can be used at a time")
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s: %s",
			sentinel, selectionScope(gatewayType, features), strings.Join(problems, "; "))
	}

	if len(presetFeatures) > 0 {
		if len(matches) > 1 {
			return nil, fmt.Errorf("%w: features %v are enabled on %d presets of type %q; exactly one is required",
				ErrAmbiguousPreset, presetFeatures, len(matches), gatewayType)
		}
		if len(matches) == 0 {
			// Unreachable: explainMissingFeatures shares supportsFeatures, so an
			// unsatisfied feature always yields a problem above. Guarded anyway
			// because the invariant spans two functions and this would panic.
			return nil, fmt.Errorf("%w: %s", ErrNoMatchingPreset, selectionScope(gatewayType, features))
		}
		return matches[0], nil
	}

	var defaults []*Preset
	for _, preset := range matches {
		if preset.Default {
			defaults = append(defaults, preset)
		}
	}
	if len(defaults) > 1 {
		return nil, fmt.Errorf("%w: %d presets of type %q are marked default; exactly one is required",
			ErrAmbiguousPreset, len(defaults), gatewayType)
	}
	if len(defaults) == 1 {
		return defaults[0], nil
	}
	return nil, fmt.Errorf("%w: no default preset for gateway type %q", ErrNoDefaultPreset, gatewayType)
}

func (s *ZoneSpec) FeaturesSupported(gatewayType GatewayType, features ...FeatureName) bool {
	_, err := s.SelectPreset(gatewayType, features...)
	return err == nil
}

// MatchingGateways returns the distinct gateways reachable for a traffic type and
// feature set. Selection is single-valued, but callers provision per physical gateway,
// so the result is deduplicated and sorted for a stable status.
func (s *ZoneSpec) MatchingGateways(gatewayType GatewayType, features ...FeatureName) ([]*GatewayConfig, error) {
	presetFeatures, problems := s.validateFeatures(features)
	if len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s: %s",
			ErrInvalidFeatures, selectionScope(gatewayType, features), strings.Join(problems, "; "))
	}

	gatewaysByName := make(map[string]*GatewayConfig)
	candidates := s.presetsOfType(gatewayType)
	for _, preset := range candidates {
		if !preset.SupportsFeatures(presetFeatures) {
			continue
		}
		gateway, err := s.GetGateway(preset.GatewayRef)
		if err != nil {
			return nil, err
		}
		gatewaysByName[gateway.Name] = gateway
	}
	if len(gatewaysByName) == 0 {
		problems = s.explainMissingFeatures(gatewayType, presetFeatures)
		if len(problems) == 0 {
			if len(candidates) == 0 {
				problems = append(problems, fmt.Sprintf("no preset of type %q exists", gatewayType))
			} else {
				// ponytail: unreachable while ConsumerFailover is the only preset-scoped feature,
				// but required so the reason is never a false "no preset of this type exists".
				problems = append(problems, "requested features are enabled on different presets, but only one preset can be used at a time")
			}
		}
		return nil, fmt.Errorf("%w: %s: %s",
			ErrNoMatchingPreset, selectionScope(gatewayType, features), strings.Join(problems, "; "))
	}

	gateways := slices.Collect(maps.Values(gatewaysByName))
	slices.SortFunc(gateways, func(a, b *GatewayConfig) int { return strings.Compare(a.Name, b.Name) })
	return gateways, nil
}

type Links struct {
	// Url is the base URL of the default gateway of this zone preset
	// +kubebuilder:validation:Format=uri
	Url string `json:"gatewayUrl"`
	// Issuer is the expected issuer of downstream tokens for this zone preset
	// +kubebuilder:validation:Format=uri
	Issuer string `json:"gatewayIssuer"`

	// TokenUrl is the token endpoint for this zone preset
	// +kubebuilder:validation:Format=uri
	TokenUrl string `json:"tokenUrl,omitempty"`

	// TeamIssuer is the expected issuer of downstream tokens for Team APIs in this zone preset
	// +kubebuilder:validation:Format=uri
	// +optional
	TeamIssuer string `json:"teamApiIssuer,omitempty"`

	// LmsIssuer is the issuer of the Last-Mile-Security tokens (upstream) for this zone preset
	// +kubebuilder:validation:Format=uri
	// +optional
	LmsIssuer string `json:"gatewayLmsIssuer"`

	// InternalIssuer is the expected issuer of downstream tokens for internal services in this zone preset
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
	Name string `json:"name"`
	// Types is the union of the types of the presets referencing this gateway.
	// +listType=set
	// +optional
	Types       []GatewayType    `json:"types,omitempty"`
	Gateway     *types.ObjectRef `json:"gateway,omitempty"`
	AdminClient *types.ObjectRef `json:"adminClient,omitempty"`
	Consumer    *types.ObjectRef `json:"consumer,omitempty"`
}

type PresetStatus struct {
	Name                string           `json:"name"`
	GatewayRef          *types.ObjectRef `json:"gatewayRef,omitempty"`
	IdentityProviderRef *types.ObjectRef `json:"identityProviderRef,omitempty"`
	Links               Links            `json:"links,omitempty"`
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
	case FeatureConsumerFailover:
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
