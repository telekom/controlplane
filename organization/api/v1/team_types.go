// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type Member struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=email
	Email string `json:"email"`
}

// TeamSpec defines the desired state of Team.
type TeamSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name is the name of the team
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=^[a-z0-9]+(-?[a-z0-9]+)*$
	Name string `json:"name"`

	// Group is the group of the team
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=^[a-z0-9]+(-?[a-z0-9]+)*$
	Group string `json:"group"`

	// Email is the mail of the team
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=email
	Email string `json:"email"`

	// Members is the members of the team
	// +kubebuilder:validation:MinItems=1
	Members []Member `json:"members"`

	// Secret for the teamToken and passed towards the identity client.
	// +kubebuilder:validation:Optional
	Secret string `json:"secret,omitempty"`
}

// TeamStatus defines the observed state of Team.
type TeamStatus struct {
	Namespace          string           `json:"namespace"`
	IdentityClientRef  *types.ObjectRef `json:"identityClientRef,omitempty"`
	GatewayConsumerRef *types.ObjectRef `json:"gatewayConsumerRef,omitempty"`
	TeamToken          string           `json:"teamToken,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

var _ types.Object = &Team{}
var _ types.ObjectList = &TeamList{}

func (t *Team) GetConditions() []metav1.Condition {
	return t.Status.Conditions
}

func (t *Team) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&t.Status.Conditions, condition)

}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Team is the Schema for the teams API.
type Team struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TeamSpec   `json:"spec,omitempty"`
	Status TeamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TeamList contains a list of Team.
type TeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Team `json:"items"`
}

func (tl *TeamList) GetItems() []types.Object {
	items := make([]types.Object, len(tl.Items))
	for i := range tl.Items {
		items[i] = &tl.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Team{}, &TeamList{})
}
