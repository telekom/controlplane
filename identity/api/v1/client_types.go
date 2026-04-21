// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClientSpec defines the desired state of Client
type ClientSpec struct {
	Realm        *types.ObjectRef `json:"realm"`
	ClientId     string           `json:"clientId"`
	ClientSecret string           `json:"clientSecret"`

	// SecretRotation controls whether this client participates in graceful secret rotation.
	// When true (and the referenced Realm has a SecretRotation config),
	// changing the client secret will preserve the old secret for the
	// configured grace period. Defaults to true (opt-out by setting to false).
	// +optional
	// +kubebuilder:default=true
	SecretRotation *bool `json:"secretRotation,omitempty"`
}

// ClientStatus defines the observed state of Client
type ClientStatus struct {
	// ClientUid is the unique identifier of the client in Keycloak. It is immutable and assigned by Keycloak upon client creation.
	ClientUid string `json:"clientUid,omitempty"`

	// IssuerUrl is the URL of the Keycloak realm's OpenID Connect issuer, which clients can use for authentication and token retrieval.
	IssuerUrl string `json:"issuerUrl"`
	// RotatedClientSecret holds the previous client secret during a graceful
	// rotation grace period. Empty when no rotation is in progress.
	// +optional
	RotatedClientSecret string `json:"rotatedClientSecret,omitempty"`

	// RotatedSecretExpiresAt indicates when the rotated (old) secret will
	// stop being accepted. Nil when no rotation is in progress.
	// +optional
	RotatedSecretExpiresAt *metav1.Time `json:"rotatedSecretExpiresAt,omitempty"`

	// SecretExpiresAt indicates when the current client secret will be
	// auto-expired by Keycloak's secret-rotation executor. Only populated
	// when secretRotation is enabled and the referenced Realm has a
	// SecretRotation config. Nil if the creation timestamp is unavailable
	// or the Realm has no SecretRotation configuration.
	// +optional
	SecretExpiresAt *metav1.Time `json:"secretExpiresAt,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Client is the Schema for the clients API
type Client struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClientSpec   `json:"spec,omitempty"`
	Status ClientStatus `json:"status,omitempty"`
}

var _ types.Object = &Client{}

// SupportsSecretRotation returns true when the client supports graceful secret
// rotation. This is the default (nil or true); only an explicit false disables it.
func (c *Client) SupportsSecretRotation() bool {
	return c.Spec.SecretRotation == nil || *c.Spec.SecretRotation
}

func (e *Client) GetConditions() []metav1.Condition {
	return e.Status.Conditions
}

func (e *Client) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&e.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ClientList contains a list of Client
type ClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Client `json:"items"`
}

var _ types.ObjectList = &ClientList{}

func (el *ClientList) GetItems() []types.Object {
	items := make([]types.Object, len(el.Items))
	for i := range el.Items {
		items[i] = &el.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Client{}, &ClientList{})
}
