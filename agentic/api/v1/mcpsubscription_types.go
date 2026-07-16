// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// McpSubscriptionSpec defines the desired state of McpSubscription.
type McpSubscriptionSpec struct {
	// BasePath references the McpServer/McpExposure via basePath.
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

// McpSubscriptionStatus defines the observed state of McpSubscription.
type McpSubscriptionStatus struct {
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

// McpSubscription is the Schema for the mcpsubscriptions API.
// It represents a request by an application to consume an MCP server
// exposed via McpExposure, with approval and access control.
type McpSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   McpSubscriptionSpec   `json:"spec,omitempty"`
	Status McpSubscriptionStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &McpSubscription{}

func (r *McpSubscription) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *McpSubscription) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

func (r *McpSubscription) HasM2M() bool {
	return r.Spec.Security != nil && r.Spec.Security.M2M != nil
}

// +kubebuilder:object:root=true

// McpSubscriptionList contains a list of McpSubscription
type McpSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []McpSubscription `json:"items"`
}

var _ ctypes.ObjectList = &McpSubscriptionList{}

func (r *McpSubscriptionList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&McpSubscription{}, &McpSubscriptionList{})
}
