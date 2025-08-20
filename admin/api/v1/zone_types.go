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

type IdentityProviderConfig struct {
	Admin IdentityProviderAdminConfig `json:"admin"`
	Url   string                      `json:"url"`
}

type GatewayAdminConfig struct {
	ClientSecret string  `json:"clientSecret"`
	Url          *string `json:"url,omitempty"`
}

type GatewayConfig struct {
	Admin GatewayAdminConfig `json:"admin"`
	Url   string             `json:"url"`
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
	GatewayUrl        string `json:"gatewayUrl"`
	GatewayIssuer     string `json:"gatewayIssuer"`
	StargateLmsIssuer string `json:"stargateLmsIssuer"`
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
