// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemoteApiSubscriptionSpec defines the desired state of RemoteApiSubscription
type RemoteApiSubscriptionSpec struct {
	// +kubebuilder:validation:Required
	// TODO: Validate if regex is correct +kubebuilder:validation:Pattern=`^\/[a-zA-Z0-9]+(\/[a-zA-Z0-9]+)*\/v[0-9]+$`
	ApiBasePath        string `json:"apiBasePath"`
	TargetOrganization string `json:"targetOrganization"`
	SourceOrganization string `json:"sourceOrganization,omitempty"`
	// Requester is the entity that is requesting the subscription
	Requester RemoteRequester `json:"requester"`
	// Security is the security configuration for the subscription
	Security *Security `json:"security,omitempty"`
}

type RemoteRequester struct {
	// Application is the name of the application that is requesting the subscription
	Application string `json:"application"`
	// Team is the team that is requesting the subscription
	Team RemoteTeam `json:"team"`
}

type RemoteTeam struct {
	// Name is the logical name of the team
	Name string `json:"name"`
	// Email is the email address of the team
	Email string `json:"email"`
}

type ApprovalInfo struct {
	ApprovalState string `json:"approvalState"` // TODO: Enum?
	Message       string `json:"message"`
}

// RemoteApiSubscriptionStatus defines the observed state of RemoteApiSubscription
type RemoteApiSubscriptionStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions      []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	GatewayUrl      string             `json:"gatewayUrl"`
	Approval        *ApprovalInfo      `json:"approval,omitempty"`
	ApprovalRequest *ApprovalInfo      `json:"approvalRequest,omitempty"`

	// Route is only present if we are the source organization
	Route *ctypes.ObjectRef `json:"route,omitempty"`
	// ApiSubscription is only present if we are the target organization
	ApiSubscription *ctypes.ObjectRef `json:"apiSubscription,omitempty"`
	// Application is only present if we are the target organization
	Application *ctypes.ObjectRef `json:"application,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RemoteApiSubscription is the Schema for the remoteapisubscriptions API
type RemoteApiSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteApiSubscriptionSpec   `json:"spec,omitempty"`
	Status RemoteApiSubscriptionStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &RemoteApiSubscription{}

func (as *RemoteApiSubscription) GetConditions() []metav1.Condition {
	return as.Status.Conditions
}

func (as *RemoteApiSubscription) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&as.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// RemoteApiSubscriptionList contains a list of RemoteApiSubscription
type RemoteApiSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteApiSubscription `json:"items"`
}

var _ ctypes.ObjectList = &RemoteApiSubscriptionList{}

func (l *RemoteApiSubscriptionList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&RemoteApiSubscription{}, &RemoteApiSubscriptionList{})
}
