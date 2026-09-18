// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RouteListenerSpec defines the desired state of RouteListener.
// RouteListener enables API traffic capture on a gateway Route for a specific consumer.
type RouteListenerSpec struct {
	// Route the listener attaches to (the API whose traffic is captured).
	Route types.ObjectRef `json:"route"`
	// Zone where listening happens.
	Zone types.ObjectRef `json:"zone"`
	// Consumer application id allowed to listen (keys the jumper routeListener map).
	Consumer string `json:"consumer"`
	// ServiceOwner = provider application id whose API is listened to.
	ServiceOwner string `json:"serviceOwner"`
	// Issue = the API basePath being listened to.
	Issue string `json:"issue"`
	// GatewayClient credentials the jumper uses to mint the publisher token.
	GatewayClient GatewayClientConfig `json:"gatewayClient"`
}

type GatewayClientConfig struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Issuer       string `json:"issuer"`
}

// RouteListenerStatus defines the observed state of RouteListener.
type RouteListenerStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RouteListener is the Schema for the routelisteners API
type RouteListener struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouteListenerSpec   `json:"spec"`
	Status RouteListenerStatus `json:"status,omitempty"`
}

var _ types.Object = &RouteListener{}

func (r *RouteListener) GetConditions() []metav1.Condition { return r.Status.Conditions }
func (r *RouteListener) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// RouteListenerList contains a list of RouteListener
type RouteListenerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RouteListener `json:"items"`
}

var _ types.ObjectList = &RouteListenerList{}

func (r *RouteListenerList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&RouteListener{}, &RouteListenerList{})
}
