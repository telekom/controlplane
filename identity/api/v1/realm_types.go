// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RealmSpec defines the desired state of Realm
type RealmSpec struct {
	IdentityProvider *types.ObjectRef `json:"identityProvider"`
}

// RealmStatus defines the observed state of Realm
type RealmStatus struct {
	IssuerUrl     string `json:"issuerUrl"`
	AdminClientId string `json:"adminClientId"`
	AdminUserName string `json:"adminUserName"`
	AdminPassword string `json:"adminPassword"`
	AdminUrl      string `json:"adminUrl"`
	AdminTokenUrl string `json:"adminTokenUrl"`
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
