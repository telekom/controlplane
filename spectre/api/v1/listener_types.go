// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

// ListenerSpec defines the desired state of Listener.
// +kubebuilder:validation:XValidation:rule="has(self.apiListener) || has(self.eventListener)",message="at least one of apiListener or eventListener must be set"
type ListenerSpec struct {
	Consumer ctypes.TypedObjectRef `json:"consumer"`
	Provider ctypes.TypedObjectRef `json:"provider"`
	// Application references the owning SpectreApplication (set by the rover producer).
	Application ctypes.ObjectRef `json:"application"`
	// +optional
	ApiListener *ApiListener `json:"apiListener,omitempty"`
	// +optional
	EventListener *EventListener `json:"eventListener,omitempty"`
}

// ApiListener configures a listener that proxies API requests.
type ApiListener struct {
	ApiBasePath string `json:"apiBasePath"`
	// +optional
	RequestFilter *ListenerFilter `json:"requestFilter,omitempty"`
	// +optional
	ResponseFilter *ListenerFilter `json:"responseFilter,omitempty"`
}

// EventListener configures a listener that subscribes to events.
type EventListener struct {
	EventType string `json:"eventType"`
	// +optional
	Filter *ListenerFilter `json:"filter,omitempty"`
}

// ListenerFilter defines trigger and payload filtering rules.
type ListenerFilter struct {
	// +optional
	Trigger map[string]string `json:"trigger,omitempty"`
	// +optional
	Payload []string `json:"payload,omitempty"`
}

// ListenerStatus defines the observed state of Listener.
type ListenerStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// +optional
	RouteListener *ctypes.ObjectRef `json:"routeListener,omitempty"`
	// +optional
	EventSubscriptions []ctypes.ObjectRef `json:"eventSubscriptions,omitempty"`
	// ProviderApproval references the Approval that gates this Listener. The
	// provider owns the API whose traffic is captured, so its consent is the
	// security control. Only one approval can be modelled per owner CR — see
	// the note on ensureApprovals.
	// +optional
	ProviderApproval *ctypes.ObjectRef `json:"providerApproval,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Listener is the Schema for the listeners API.
type Listener struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ListenerSpec `json:"spec"`
	// +optional
	Status ListenerStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &Listener{}

func (l *Listener) GetConditions() []metav1.Condition {
	return l.Status.Conditions
}

//nolint:gocritic // hugeParam: kept as value to satisfy ctypes.Object interface
func (l *Listener) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&l.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ListenerList contains a list of Listener.
type ListenerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Listener `json:"items"`
}

var _ ctypes.ObjectList = &ListenerList{}

// GetItems implements types.ObjectList. The janitor client requires this to
// clean up owned Listeners; without it CleanupAll aborts for every type.
func (l *ListenerList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Listener{}, &ListenerList{})
}
