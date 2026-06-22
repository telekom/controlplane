// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RouteType defines the type of the route.
// +kubebuilder:validation:Enum=primary;secondary;proxy
type RouteType string

const (
	// RouteTypePrimary is the primary route that is the main egress point for all traffic on this Route.
	// It is the target of proxy routes.
	RouteTypePrimary RouteType = "primary"
	// RouteTypeSecondary is the failover route in case the primary route is not available.
	// It becomes the target of proxy routes if the primary route is not available.
	RouteTypeSecondary RouteType = "secondary"
	// RouteTypeProxy is the route that enables meshing between Gateways by proxying all requests to either
	// the primary or the secondary route. It never sends traffic to the upstream directly.
	RouteTypeProxy RouteType = "proxy"
)

type Backend struct {
	// Upstreams defines the upstream targets for this route. If multiple targets are defined, they will be load balanced according to their weight.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=10
	Upstreams []Upstream `json:"upstreams"`
}

// RouteSpec defines the desired state of Route
type RouteSpec struct {
	// GatewayRef is a reference to the Gateway this Route belongs to
	// +kubebuilder:validation:Required
	GatewayRef types.ObjectRef `json:"gatewayRef"`

	// Type defines the type of the route. It can be either primary, secondary or proxy. The default is primary.
	// +kubebuilder:default=primary
	Type RouteType `json:"type"`

	// Hostnames defines the hostnames that are accepted for this route. If empty, all hostnames are accepted.
	// +listType=set
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=20
	// +kubebuilder:validation:items:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self.all(h, !format.dns1123Subdomain().validate(h).hasValue())",message="each hostname must be a valid DNS-1123 subdomain"
	Hostnames []string `json:"hostnames,omitempty"`

	// Backend defines the backend for this route. Only one of Backend or Traffic can be set.
	// +kubebuilder:validation:Required
	Backend Backend `json:"backend"`

	// Paths defines the paths that are accepted for this route. If empty, all paths are accepted.
	// +listType=set
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	Paths []string `json:"paths,omitempty"`

	// PassThrough is a flag to pass through the request to the upstream without authentication
	// +kubebuilder:default=false
	PassThrough bool `json:"passThrough"`

	// Traffic defines the traffic configuration for this route.
	Traffic Traffic `json:"traffic"`

	// Security is the security configuration for the route
	// +kubebuilder:validation:Optional
	Security Security `json:"security,omitempty"`

	// Transformation defines optional request/response transformations for this API
	// +kubebuilder:validation:Optional
	Transformation *Transformation `json:"transformation,omitempty"`

	// Buffering configures Kong request/response body buffering for this route
	Buffering Buffering `json:"buffering,omitempty"`
}

// RouteStatus defines the observed state of Route.
type RouteStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// +optional
	// +kubebuilder:validation:Type=array
	// +kubebuilder:validation:items:Type=string
	Consumers  []string          `json:"consumers,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Route is the Schema for the routes API
type Route struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Route
	// +required
	Spec RouteSpec `json:"spec"`

	// status defines the observed state of Route
	// +optional
	Status RouteStatus `json:"status,omitzero"`
}

var _ types.Object = &Route{}

func (g *Route) GetConditions() []metav1.Condition {
	return g.Status.Conditions
}

func (g *Route) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&g.Status.Conditions, condition)
}

func (g *Route) SetProperty(key, val string) {
	if g.Status.Properties == nil {
		g.Status.Properties = make(map[string]string)
	}
	g.Status.Properties[key] = val
}

func (g *Route) GetProperty(key string) string {
	if g.Status.Properties == nil {
		return ""
	}
	val := g.Status.Properties[key]
	return val
}

func (g *Route) IsProxy() bool {
	return g.Spec.Type != RouteTypePrimary
}

func (g *Route) IsFailoverSecondary() bool {
	return g.Spec.Type == RouteTypeSecondary
}

func (g *Route) GetTrustedIssuers() []string {
	return g.Spec.Security.TrustedIssuers
}

// GetHostnames implements the CustomRoute interface for Route
func (g *Route) GetHostnames() []string {
	return g.Spec.Hostnames
}

// GetPaths implements the CustomRoute interface for Route
func (g *Route) GetPaths() []string {
	return g.Spec.Paths
}

// GetRequestBuffering implements the CustomRoute interface for Route
func (g *Route) GetRequestBuffering() bool {
	return !g.Spec.Buffering.DisableRequestBuffering
}

// GetResponseBuffering implements the CustomRoute interface for Route
func (g *Route) GetResponseBuffering() bool {
	return !g.Spec.Buffering.DisableResponseBuffering
}

func (g *Route) SetRouteId(id string) {
	g.SetProperty("routeId", id)
}

func (g *Route) SetServiceId(id string) {
	g.SetProperty("serviceId", id)
}

func (g *Route) SetUpstreamId(id string) {
	g.SetProperty("upstreamId", id)
}

func (g *Route) SetTargetsId(id string) {
	g.SetProperty("targetsId", id)
}

func (g *Route) GetUpstreamId() string {
	return g.GetProperty("upstreamId")
}

func (g *Route) GetTargetsId() string {
	return g.GetProperty("targetsId")
}

// +kubebuilder:object:root=true

// RouteList contains a list of Route
type RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Route `json:"items"`
}

var _ types.ObjectList = &RouteList{}

func (r *RouteList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Route{}, &RouteList{})
}
