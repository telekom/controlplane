// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApiSubscriptionSpec defines the desired state of ApiSubscription
type ApiSubscriptionSpec struct {
	ApiBasePath  string           `json:"apiBasePath"`
	Security     *Security        `json:"security,omitempty"`
	Organization string           `json:"organization,omitempty"`
	Requestor    Requestor        `json:"requestor"`
	Zone         ctypes.ObjectRef `json:"zone"`
}

type Requestor struct {
	Application ctypes.ObjectRef `json:"application"`
}

type Security struct {
	Oauth2Scopes []string `json:"oauth2Scopes,omitempty"`
}

// ApiSubscriptionStatus defines the observed state of ApiSubscription
type ApiSubscriptionStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	Route                 *ctypes.ObjectRef `json:"route,omitempty"`
	ConsumeRoute          *ctypes.ObjectRef `json:"consumeRoute,omitempty"`
	Approval              *ctypes.ObjectRef `json:"approval,omitempty"`
	ApprovalRequest       *ctypes.ObjectRef `json:"approvalRequest,omitempty"`
	RemoteApiSubscription *ctypes.ObjectRef `json:"remoteApiSubscription,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ApiSubscription is the Schema for the apisubscriptions API
type ApiSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApiSubscriptionSpec   `json:"spec,omitempty"`
	Status ApiSubscriptionStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &ApiSubscription{}

func (as *ApiSubscription) GetConditions() []metav1.Condition {
	return as.Status.Conditions
}

func (as *ApiSubscription) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&as.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ApiSubscriptionList contains a list of ApiSubscription
type ApiSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiSubscription `json:"items"`
}

var _ ctypes.ObjectList = &ApiSubscriptionList{}

func (l *ApiSubscriptionList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ApiSubscription{}, &ApiSubscriptionList{})
}
