// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgenticSubscriptionSpec defines the desired state of AgenticSubscription.
type AgenticSubscriptionSpec struct {
	// BasePath references the AgenticServer/AgenticExposure via basePath.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^/.*$`
	BasePath string `json:"basePath"`

	// Requestor identifies the application requesting access to the MCP server.
	Requestor Requestor `json:"requestor"`

	// TODO: Consider adding Organization field for cross-org MCP subscriptions (remote subscription flow).
	// See api domain's ApiSubscriptionSpec.Organization and RemoteApiSubscription handler for reference.

	// Zone references the Zone CR where the subscriber resides.
	Zone ctypes.ObjectRef `json:"zone"`

	// Security configures optional security settings for the ConsumeRoute.
	// +optional
	Security *SubscriberSecurity `json:"security,omitempty"`

	// Traffic configures traffic management for the ConsumeRoute.
	// +optional
	Traffic SubscriberTraffic `json:"traffic"`
}

// Requestor identifies the requesting application.
type Requestor struct {
	Application ctypes.ObjectRef `json:"application"`
}

// AgenticSubscriptionStatus defines the observed state of AgenticSubscription.
type AgenticSubscriptionStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// ConsumeRoute references the ConsumeRoute CR created for this subscription.
	// +optional
	ConsumeRoute *ctypes.ObjectRef `json:"consumeRoute,omitempty"`

	// Approval references the Approval CR for this subscription.
	// +optional
	Approval *ctypes.ObjectRef `json:"approval,omitempty"`

	// ApprovalRequest references the ApprovalRequest CR for this subscription.
	// +optional
	ApprovalRequest *ctypes.ObjectRef `json:"approvalRequest,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BasePath",type="string",JSONPath=".spec.basePath",description="The MCP server base path"
// +kubebuilder:printcolumn:name="Zone",type="string",JSONPath=".spec.zone.name",description="The subscriber zone"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AgenticSubscription is the Schema for the agenticsubscriptions API.
// It represents a request by an application to consume an MCP server
// exposed via AgenticExposure, with approval and access control.
type AgenticSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgenticSubscriptionSpec   `json:"spec,omitempty"`
	Status AgenticSubscriptionStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &AgenticSubscription{}

func (r *AgenticSubscription) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *AgenticSubscription) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

func (r *AgenticSubscription) HasM2M() bool {
	return r.Spec.Security != nil && r.Spec.Security.M2M != nil
}

// +kubebuilder:object:root=true

// AgenticSubscriptionList contains a list of AgenticSubscription
type AgenticSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgenticSubscription `json:"items"`
}

var _ ctypes.ObjectList = &AgenticSubscriptionList{}

func (r *AgenticSubscriptionList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&AgenticSubscription{}, &AgenticSubscriptionList{})
}
