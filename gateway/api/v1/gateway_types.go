// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RedisConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
}

type AdminConfig struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	IssuerUrl    string `json:"issuerUrl"`
	Url          string `json:"url"`
}

type CircuitBreaker struct {
	Passive PassiveHealthcheck `json:"passive"`
	Active  ActiveHealthCheck  `json:"active"`
}

type PassiveHealthcheck struct {
	HealthyHttpStatuses   []int `json:"healthyHttpStatuses"`
	HealthySuccesses      int   `json:"healthySuccesses"`
	UnhealthyHttpFailures int   `json:"unhealthyHttpFailures"`
	UnhealthyHttpStatuses []int `json:"unhealthyHttpStatuses"`
	UnhealthyTcpFailures  int   `json:"unhealthyTcpFailures"`
	UnhealthyTimeouts     int   `json:"unhealthyTimeouts"`
}

type ActiveHealthCheck struct {
	HealthyHttpStatuses   []int `json:"healthyHttpStatuses"`
	UnhealthyHttpStatuses []int `json:"unhealthyHttpStatuses"`
}

// GatewaySpec defines the desired state of Gateway
type GatewaySpec struct {
	Redis RedisConfig `json:"redis,omitempty"`
	Admin AdminConfig `json:"admin,omitempty"`

	Features []FeatureType `json:"features,omitempty"`

	CircuitBreaker *CircuitBreaker `json:"circuitBreaker,omitempty"`
}

// GatewayStatus defines the observed state of Gateway
type GatewayStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Gateway is the Schema for the gateways API
type Gateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewaySpec   `json:"spec,omitempty"`
	Status GatewayStatus `json:"status,omitempty"`
}

var _ types.Object = &Gateway{}

func (g *Gateway) Admin() AdminConfig {
	return g.Spec.Admin
}

func (g *Gateway) AdminUrl() string {
	return g.Spec.Admin.Url
}

func (g *Gateway) AdminClientId() string {
	return g.Spec.Admin.ClientId
}

func (g *Gateway) AdminClientSecret() string {
	return g.Spec.Admin.ClientSecret
}

func (g *Gateway) AdminIssuer() string {
	return g.Spec.Admin.IssuerUrl
}

func (g *Gateway) GetConditions() []metav1.Condition {
	return g.Status.Conditions
}

func (g *Gateway) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&g.Status.Conditions, condition)
}

func (g *Gateway) SupportsFeature(feature FeatureType) bool {
	for _, f := range g.Spec.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// +kubebuilder:object:root=true

// GatewayList contains a list of Gateway
type GatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gateway `json:"items"`
}

var _ types.ObjectList = &GatewayList{}

func (g *GatewayList) GetItems() []types.Object {
	items := make([]types.Object, len(g.Items))
	for i := range g.Items {
		items[i] = &g.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Gateway{}, &GatewayList{})
}
