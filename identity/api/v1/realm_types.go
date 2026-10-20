// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	types "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretRotationConfig holds configuration for Keycloak's graceful
// client-secret rotation policy applied at realm level.
type SecretRotationConfig struct {
	// GracePeriod is how long the old client secret remains valid after
	// rotation, specified as a duration string (e.g. "1h", "300s", "30m").
	// Maps to Keycloak's "rotated-expiration-period".
	// +kubebuilder:validation:Required
	GracePeriod metav1.Duration `json:"gracePeriod"`

	// ExpirationPeriod is how long a client secret is valid before it must
	// be rotated, specified as a duration string (e.g. "696h", "2505600s").
	// Maps to Keycloak's "expiration-period".
	// +kubebuilder:validation:Required
	ExpirationPeriod metav1.Duration `json:"expirationPeriod"`

	// RemainingRotationPeriod is the window before secret expiry during
	// which rotation is allowed, specified as a duration string (e.g. "240h", "864000s").
	// Maps to Keycloak's "remaining-rotation-period".
	//
	// Keycloak blocks rotation attempts that fall outside this window. For
	// example, if ExpirationPeriod is 29 days and RemainingRotationPeriod
	// is 10 days, rotation is only permitted in the last 10 days before
	// expiry. Setting RemainingRotationPeriod equal to ExpirationPeriod
	// allows rotation at any time (recommended, since the controller
	// drives rotation externally rather than relying on Keycloak's
	// expiration timer).
	// +kubebuilder:validation:Required
	RemainingRotationPeriod metav1.Duration `json:"remainingRotationPeriod"`
}

// ClaimType specifies the kind of protocol mapper used for a claim.
// +kubebuilder:validation:Enum=HardcodedClaim;SessionNote
type ClaimType string

const (
	// ClaimTypeHardcodedClaim injects a static value into every token.
	ClaimTypeHardcodedClaim ClaimType = "HardcodedClaim"

	// ClaimTypeSessionNote reads the claim value from a Keycloak user-session
	// note (e.g. "clientId", "clientHost", "clientAddress"). The value is
	// populated automatically by Keycloak during authentication.
	ClaimTypeSessionNote ClaimType = "SessionNote"
)

// ClaimConfig defines a claim that is added to all tokens issued for
// clients in this realm.
type ClaimConfig struct {
	// Name is the claim name as it appears in the token (e.g. "team", "env").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is the static claim value written into every token.
	// Only used for HardcodedClaim; ignored for SessionNote (which
	// reads the value from the Keycloak session note keyed by Name).
	// +optional
	Value string `json:"value,omitempty"`

	// Type selects the protocol mapper used for this claim.
	// Defaults to HardcodedClaim when omitted.
	// +optional
	// +kubebuilder:default=HardcodedClaim
	Type ClaimType `json:"type,omitempty"`
}

// RealmSpec defines the desired state of Realm
type RealmSpec struct {
	IdentityProvider *types.ObjectRef `json:"identityProvider"`
	// AllowedOrigins is inherited by clients in this realm.
	// +optional
	AllowedOrigins []string `json:"allowedOrigins,omitempty"`

	// SecretRotation configures the Keycloak client-secret rotation policy
	// for this realm. When set, the controller ensures a client-policy
	// profile + policy with the given grace period exists in Keycloak.
	// When nil, the controller does not manage rotation policy.
	// +optional
	SecretRotation *SecretRotationConfig `json:"secretRotation,omitempty"`

	// Claims defines claims that are added to all tokens issued for
	// clients in this realm. Each claim can be a static value
	// (HardcodedClaim) or derived from a Keycloak session note
	// (SessionNote). The controller manages a dedicated Keycloak
	// client scope with protocol mappers for each claim.
	// +optional
	// +listType=map
	// +listMapKey=name
	Claims []ClaimConfig `json:"claims,omitempty"`

	// Attributes are Keycloak realm attributes managed by the controller.
	// Keys and values are passed to Keycloak unchanged. Attributes that were
	// never listed here (e.g. configured manually in Keycloak) are preserved.
	// Removing a key from this map removes it from the Keycloak realm, but
	// only if the controller applied it before (see status.managedAttributes).
	//
	// The key "frontendUrl" sets the realm frontend URL, an absolute http(s)
	// base URL (e.g. "https://iam.example.com/auth"). It is not validated on
	// admission. If it is a valid base URL, the realm issuer is
	// "<frontendUrl>/realms/<realm>"; otherwise the issuer is read from
	// Keycloak's OIDC discovery document.
	//
	// Do not store credentials or other secrets in attributes.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(k, size(k) > 0)",message="attribute keys must not be empty"
	Attributes map[string]string `json:"attributes,omitempty"`
}

// RealmConnection describes the IdentityProvider connection of a Realm.
type RealmConnection struct {
	// IdentityProvider identifies the IdentityProvider including its UID.
	// +kubebuilder:validation:Required
	IdentityProvider types.ObjectRef `json:"identityProvider"`
	// TLS references the CA bundle trusted for the connection. Absent means system roots only.
	// +optional
	TLS *RealmConnectionTLS `json:"tls,omitempty"`
}

// RealmConnectionTLS references a CA bundle together with the namespace it is read from.
type RealmConnectionTLS struct {
	// Namespace of the referenced ConfigMap, the namespace of the IdentityProvider.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
	// ConfigMapRef references the PEM encoded CA certificates.
	// +kubebuilder:validation:Required
	ConfigMapRef types.ConfigMapKeyRef `json:"configMapRef"`
}

// RealmStatus defines the observed state of Realm
type RealmStatus struct {
	IssuerUrl     string `json:"issuerUrl"`
	AdminClientId string `json:"adminClientId"`
	AdminUserName string `json:"adminUserName"`
	AdminPassword string `json:"adminPassword"`
	AdminUrl      string `json:"adminUrl"`
	AdminTokenUrl string `json:"adminTokenUrl"`

	// ManagedAttributes is the sorted list of realm attribute keys that the
	// controller has applied to Keycloak. Keys in this list that are no
	// longer present in spec.attributes are removed from Keycloak.
	// +optional
	// +listType=set
	ManagedAttributes []string `json:"managedAttributes,omitempty"`
	// Connection identifies the IdentityProvider and CA bundle the admin connection
	// fields above were derived from. Absent for realms reconciled before it was introduced.
	// +optional
	Connection *RealmConnection `json:"connection,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Realm is the Schema for the realms API
type Realm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RealmSpec   `json:"spec,omitempty"`
	Status RealmStatus `json:"status,omitempty"`
}

var _ types.Object = &Realm{}

func (e *Realm) GetConditions() []metav1.Condition {
	return e.Status.Conditions
}

func (e *Realm) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&e.Status.Conditions, condition)
}

func (e *Realm) SupportsGracefulSecretRotation() bool {
	return e.Spec.SecretRotation != nil
}

// +kubebuilder:object:root=true

// RealmList contains a list of Realm
type RealmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Realm `json:"items"`
}

var _ types.ObjectList = &RealmList{}

func (el *RealmList) GetItems() []types.Object {
	items := make([]types.Object, len(el.Items))
	for i := range el.Items {
		items[i] = &el.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Realm{}, &RealmList{})
}
