// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ZoneVisibility string

const (
	ZoneVisibilityWorld      ZoneVisibility = "World"
	ZoneVisibilityEnterprise ZoneVisibility = "Enterprise"
)

type RedisConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Password  string `json:"password"`
	EnableTLS bool   `json:"enableTLS"`
}

type IdentityProviderAdminConfig struct {
	Url      *string `json:"url,omitempty"`
	ClientId string  `json:"clientId"`
	UserName string  `json:"userName"`
	Password string  `json:"password"`
}

type SecretRotationConfig struct {
	// Enabled controls whether secret rotation is enabled for this zone.
	//  If false, secrets will not be rotated and the grace period and expiration period will be ignored.
	Enabled bool `json:"enabled"`
	// GracePeriod is the duration that a rotated secret is valid for.
	// This allows to have a smooth transition when rotating secrets, as the old secret is still valid for a certain period of time after rotation.
	// +kubebuilder:validation:Required
	GracePeriod metav1.Duration `json:"gracePeriod"`

	// RotationInterval is the duration after which secrets should be rotated.
	// This is the interval at which the rotation process will be triggered.
	// +kubebuilder:validation:Required
	RotationInterval metav1.Duration `json:"rotationInterval"`
}

type IdentityProviderConfig struct {
	Admin IdentityProviderAdminConfig `json:"admin"`
	Url   string                      `json:"url"`

	// SecretRotation contains the config for rotating secrets related to the default identity provider realm of this zone.
	// If not set, secret rotation will be disabled.
	SecretRotation *SecretRotationConfig `json:"secretRotation,omitempty"`
}

type GatewayAdminConfig struct {
	ClientSecret string  `json:"clientSecret"`
	Url          *string `json:"url,omitempty"`
}

type GatewayConfig struct {
	Admin GatewayAdminConfig `json:"admin"`
	Url   string             `json:"url"`
	// CircuitBreaker flag that controls if circuit breaker should be enabled on this zone. the config of the CB itself comes from hardcoded values, not configurable
	CircuitBreaker bool `json:"circuitBreaker"`
}

type ApiConfig struct {
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
}

type TeamApiConfig struct {
	Apis []ApiConfig `json:"apis"`
}

// ZoneSpec defines the desired state of Zone
type ZoneSpec struct {
	IdentityProvider IdentityProviderConfig `json:"identityProvider"`
	Gateway          GatewayConfig          `json:"gateway"`
	Redis            RedisConfig            `json:"redis"`
	TeamApis         *TeamApiConfig         `json:"teamApis,omitempty"`
	// +kubebuilder:validation:Enum=World;Enterprise
	// Visibility controls what subscriptions are allowed from and to this zone. It's also relevant for features like failover
	Visibility ZoneVisibility `json:"visibility"`
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
}

// ZoneStatus defines the observed state of Zone
type ZoneStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	Namespace        string           `json:"namespace,omitempty"`
	IdentityProvider *types.ObjectRef `json:"identityProvider,omitempty"`
	IdentityRealm    *types.ObjectRef `json:"identityRealm,omitempty"`

	Gateway         *types.ObjectRef `json:"gateway,omitempty"`
	GatewayRealm    *types.ObjectRef `json:"gatewayRealm,omitempty"`
	GatewayClient   *types.ObjectRef `json:"gatewayClient,omitempty"`
	GatewayConsumer *types.ObjectRef `json:"gatewayConsumer,omitempty"`

	TeamApiIdentityRealm *types.ObjectRef  `json:"teamApiIdentityRealm,omitempty"`
	TeamApiGatewayRealm  *types.ObjectRef  `json:"teamApiGatewayRealm,omitempty"`
	TeamApiRoutes        []types.ObjectRef `json:"teamApiRoutes,omitempty"`
	Links                Links             `json:"links,omitempty"`

	// Features is a list of features that are enabled or disabled for this zone.
	// This can be used to control the availability of certain features in the zone
	// +listType=map
	// +listMapKey=name
	// +patchStrategy=merge
	// +patchMergeKey=name
	// +optional
	Features []Feature `json:"features,omitempty"`
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
	// FeatureSecretRotation indicates that secret rotation is enabled for the zone.
	FeatureSecretRotation FeatureName = "SecretRotation"
)

type Feature struct {
	Name    FeatureName `json:"name"`
	Enabled bool        `json:"enabled"`
}

func (z *Zone) IsFeatureEnabled(featureName FeatureName) bool {
	for _, feature := range z.Status.Features {
		if feature.Name == featureName {
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
