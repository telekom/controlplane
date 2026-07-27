// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"slices"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdminConfig configures the connection to the configuration backend.
type AdminConfig struct {
	// Url is the base URL of the configuration backend API.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=uri
	Url string `json:"url"`

	// Client configures the identity client used for admin access to the configuration backend.
	// If omitted, defaults are applied: clientId and clientSecret are auto-generated,
	// and the realm is resolved from the Zone's internal identity realm.
	// +optional
	Client ClientConfig `json:"client,omitempty"`
}

type ClientConfig struct {
	// ClientId is the OAuth2 client ID for authentication
	// If empty, a default client ID will be used.
	ClientId string `json:"clientId,omitempty"`

	// ClientSecret is the OAuth2 client secret for authentication
	// If empty, a new secret will be generated and a reference to it will be stored in the EventConfig status.
	ClientSecret string `json:"clientSecret,omitempty"`

	// Realm references the identity Realm CR used for OAuth2 authentication
	// If empty, it is assumed that the default realm should be used.
	Realm ctypes.ObjectRef `json:"realm,omitempty"`
}

// MeshConfig configures the mesh topology for event distribution.
// Either FullMesh can be enabled for a full mesh topology, or specific ZoneNames can be listed for a partial mesh.
type MeshConfig struct {
	// FullMesh enables a full mesh topology where events are distributed to all zones.
	// +kubebuilder:default=true
	FullMesh bool `json:"fullMesh"`
	// ZoneNames lists specific zones for event distribution in a partial mesh topology.
	// Must be set if FullMesh is false.
	// +optional
	ZoneNames []string `json:"zoneNames,omitempty"`

	// Client configures the identity client used for mesh communication between zones.
	// If omitted, defaults are applied: clientId and clientSecret are auto-generated,
	// and the realm is resolved from the Zone's default identity realm.
	// +optional
	Client ClientConfig `json:"client,omitempty"`
}

// LocalBackend configures a zone that runs its own event backend (Horizon).
// It holds the connection to the configuration backend and the internal
// upstream URLs for the SSE, publish, and Voyager gateway Routes.
type LocalBackend struct {
	// Admin configures the connection to the configuration backend.
	Admin AdminConfig `json:"admin"`

	// ServerSendEventUrl is the internal URL of the SSE backend service
	// Used as the upstream for the SSE gateway Route.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=uri
	ServerSendEventUrl string `json:"serverSendEventUrl"`

	// PublishEventUrl is the internal URL of the publish backend service
	// Used as the upstream for the publish gateway Route.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=uri
	PublishEventUrl string `json:"publishEventUrl"`

	// VoyagerApiUrl is the internal URL of the Voyager backend service.
	// Used as the upstream for the Voyager gateway Route which exposes
	// event listing and redelivery APIs.
	// +kubebuilder:validation:Format=uri
	// +optional
	VoyagerApiUrl string `json:"voyagerApiUrl,omitempty"`
}

// ProxyBackend configures a zone that runs no local event backend (no Horizon).
// All event traffic and configuration is proxied to the single TargetZone,
// which must be a local (non-proxy) zone running Horizon.
type ProxyBackend struct {
	// TargetZone references the local zone whose event backend this proxy zone
	// forwards all publish, subscribe, and configuration traffic to.
	TargetZone ctypes.ObjectRef `json:"targetZone"`
}

// EventConfigSpec defines the desired state of EventConfig.
// Exactly one of Local or Proxy must be set: Local for a zone running its own
// event backend (Horizon), Proxy for a zone that forwards all traffic to a target zone.
// +kubebuilder:validation:XValidation:rule="has(self.local) != has(self.proxy)",message="exactly one of spec.local or spec.proxy must be set"
type EventConfigSpec struct {
	// Zone references the Zone for which this EventConfig applies.
	Zone ctypes.ObjectRef `json:"zone"`

	// Local configures a zone that runs its own event backend (Horizon).
	// Mutually exclusive with Proxy.
	// +optional
	Local *LocalBackend `json:"local,omitempty"`

	// Proxy configures a zone that runs no local event backend and forwards
	// all traffic to a target zone. Mutually exclusive with Local.
	// +optional
	Proxy *ProxyBackend `json:"proxy,omitempty"`

	// Mesh configures the mesh topology for event distribution.
	// If omitted, defaults to full mesh with the realm resolved from the Zone's default identity realm.
	// +optional
	Mesh *MeshConfig `json:"mesh,omitempty"`
}

// EventConfigStatus defines the observed state of EventConfig.
type EventConfigStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// EventStore references the EventStore CR in the pubsub domain.
	// +optional
	EventStore *ctypes.ObjectRef `json:"eventStore,omitempty"`

	// AdminClient references the identity Client CR created for admin access to the configuration backend.
	// +optional
	AdminClient *ObservedObjectRef `json:"adminClient,omitempty"`

	// MeshClient references the identity Client CR created for mesh access between zones.
	// +optional
	MeshClient *ObservedObjectRef `json:"meshClient,omitempty"`

	// PublishRoute references the Route CR created for the publish gateway.
	// +optional
	PublishRoute *ctypes.ObjectRef `json:"publishRoute,omitempty"`

	// PublishURL is the external URL of the publish gateway, used by event producers to publish events.
	// +optional
	PublishURL string `json:"publishUrl,omitempty"`

	// CallbackRoute references the Route CR created for the callback gateway.
	// +optional
	CallbackRoute *ctypes.ObjectRef `json:"callbackRoute,omitempty"`

	// CallbackURL is the external URL of the callback gateway, used to send events to event consumers.
	// +optional
	CallbackURL string `json:"callbackUrl,omitempty"`

	// ProxyCallbackRoutes references the Route CRs created for the proxy callback gateway.
	// +optional
	ProxyCallbackRoutes []ctypes.ObjectRef `json:"proxyCallbackRoutes,omitempty"`

	// ProxyCallbackURLs maps zone names to the external URLs of the proxy callback gateway Routes for those zones.
	// Used to send events to event consumers in other zones.
	// +optional
	ProxyCallbackURLs map[string]string `json:"proxyCallbackUrls,omitempty"`

	// VoyagerRoute references the primary Route CR created for the Voyager gateway.
	// +optional
	VoyagerRoute *ctypes.ObjectRef `json:"voyagerRoute,omitempty"`

	// VoyagerURL is the external gateway URL for the Voyager API,
	// used for event listing and redelivery.
	// +optional
	VoyagerURL string `json:"voyagerUrl,omitempty"`

	// ProxyVoyagerRoutes references the proxy Route CRs created for cross-zone Voyager access.
	// +optional
	ProxyVoyagerRoutes []ctypes.ObjectRef `json:"proxyVoyagerRoutes,omitempty"`

	// ProxyVoyagerURLs maps zone names to the external URLs of the proxy Voyager gateway Routes for those zones.
	// +optional
	ProxyVoyagerURLs map[string]string `json:"proxyVoyagerUrls,omitempty"`
}

type ObservedObjectRef struct {
	ctypes.ObjectRef `json:",inline"`

	// ObservedGeneration is the generation of the referenced object that has been observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration"`
}

func NewObservedObjectRef(obj ctypes.Object) *ObservedObjectRef {
	if obj == nil {
		return nil
	}
	return &ObservedObjectRef{
		ObjectRef:          *ctypes.ObjectRefFromObject(obj),
		ObservedGeneration: obj.GetGeneration(),
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Zone",type="string",JSONPath=".spec.zone.name",description="Zone"

// EventConfig is the Schema for the eventconfigs API.
// It provides configuration for the event operator, including the configuration backend
// connection and OAuth2 authentication settings.
type EventConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventConfigSpec   `json:"spec,omitempty"`
	Status EventConfigStatus `json:"status,omitempty"`
}

var _ ctypes.Object = &EventConfig{}

func (r *EventConfig) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *EventConfig) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// IsProxy reports whether this EventConfig is for a proxy zone that runs no
// local event backend and forwards all traffic to its target zone.
func (r *EventConfig) IsProxy() bool {
	return r.Spec.Proxy != nil
}

// IsLocal reports whether this EventConfig is for a local zone that runs its
// own event backend. The CEL XOR rule on the spec guarantees exactly one of
// local/proxy is set, so IsLocal is the negation of IsProxy for an admitted
// object; it also guards against a nil Spec.Local when neither is set.
func (r *EventConfig) IsLocal() bool {
	return r.Spec.Local != nil
}

func (r *EventConfig) SupportsZone(zoneName string) bool {
	if r.Spec.Zone.Name == zoneName {
		return true
	}
	if r.Spec.Mesh == nil || r.Spec.Mesh.FullMesh {
		return true
	}
	return slices.Contains(r.Spec.Mesh.ZoneNames, zoneName)
}

// +kubebuilder:object:root=true

// EventConfigList contains a list of EventConfig
type EventConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventConfig `json:"items"`
}

var _ ctypes.ObjectList = &EventConfigList{}

func (r *EventConfigList) GetItems() []ctypes.Object {
	items := make([]ctypes.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&EventConfig{}, &EventConfigList{})
}
