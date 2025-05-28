// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemoteOrganizationSpec defines the desired state of RemoteOrganization
type RemoteOrganizationSpec struct {
	Id           string          `json:"id"`
	Url          string          `json:"url"`
	ClientId     string          `json:"clientId"`
	ClientSecret string          `json:"clientSecret"`
	IssuerUrl    string          `json:"issuerUrl"`
	Zone         types.ObjectRef `json:"zone"`
}

// RemoteOrganizationStatus defines the observed state of RemoteOrganization
type RemoteOrganizationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Namespace which contains all resources of the RemoteOrganization
	Namespace string `json:"namespace"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RemoteOrganization is the Schema for the remoteorganizations API
type RemoteOrganization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteOrganizationSpec   `json:"spec,omitempty"`
	Status RemoteOrganizationStatus `json:"status,omitempty"`
}

var _ types.Object = &RemoteOrganization{}

func (o *RemoteOrganization) GetConditions() []metav1.Condition {
	return o.Status.Conditions
}

func (o *RemoteOrganization) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&o.Status.Conditions, condition)
}

func (o *RemoteOrganization) GetUrl() string {
	return o.Spec.Url
}

func (o *RemoteOrganization) GetClientId() string {
	return o.Spec.ClientId
}

func (o *RemoteOrganization) GetClientSecret() string {
	return o.Spec.ClientSecret
}

func (o *RemoteOrganization) GetIssuerUrl() string {
	return o.Spec.IssuerUrl
}

// +kubebuilder:object:root=true

// RemoteOrganizationList contains a list of RemoteOrganization
type RemoteOrganizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteOrganization `json:"items"`
}

var _ types.ObjectList = &RemoteOrganizationList{}

func (l *RemoteOrganizationList) GetItems() []types.Object {
	items := make([]types.Object, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&RemoteOrganization{}, &RemoteOrganizationList{})
}
