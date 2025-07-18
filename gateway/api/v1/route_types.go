// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"slices"
	"strconv"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Upstream struct {
	Weight       int    `json:"weight,omitempty"`
	Scheme       string `json:"scheme"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Path         string `json:"path"`
	IssuerUrl    string `json:"issuerUrl,omitempty"`
	ClientId     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

func (u Upstream) GetScheme() string {
	return u.Scheme
}

func (u Upstream) GetHost() string {
	return u.Host
}

func (u Upstream) GetPort() int {
	return u.Port
}

func (u Upstream) GetPath() string {
	return u.Path
}

func (u Upstream) Url() string {
	return u.Scheme + "://" + u.Host + ":" + strconv.Itoa(u.Port) + u.Path
}

// IsProxy checks if the upstream is a proxy
// In most cases a proxy-upstream is identified by having an IssuerUrl set.
func (u Upstream) IsProxy() bool {
	return u.IssuerUrl != ""
}

type Downstream struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
	IssuerUrl string `json:"issuerUrl,omitempty"`
}

// GetUrl returns the complete URL consiting of Host, Port and Path
// The scheme is always "https"
func (d Downstream) Url() string {
	return "https://" + d.Host + ":" + strconv.Itoa(d.Port) + d.Path
}

// RouteSpec defines the desired state of Route
type RouteSpec struct {
	Realm types.ObjectRef `json:"realm"`
	// PassThrough is a flag to pass through the request to the upstream without authentication
	// +kubebuilder:default=false
	PassThrough bool         `json:"passThrough"`
	Upstreams   []Upstream   `json:"upstreams"`
	Downstreams []Downstream `json:"downstreams"`

	Traffic Traffic `json:"traffic"`

	// Transformation defines optional request/response transformations for this API
	// +kubebuilder:validation:Optional
	Transformation *Transformation `json:"transformation,omitempty"`

	// Security is the security configuration for the route
	// +kubebuilder:validation:Optional
	Security *Security `json:"security,omitempty"`
}

func (route *Route) HasM2M() bool {
	if route.Spec.Security == nil {
		return false
	}
	return route.Spec.Security.M2M != nil
}

func (route *Route) HasM2MExternalIdp() bool {
	if !route.HasM2M() {
		return false
	}
	return route.Spec.Security.M2M.ExternalIDP != nil
}

func (route *Route) HasM2MExternalIdpClient() bool {
	if !route.HasM2M() {
		return false
	}
	if !route.HasM2MExternalIdp() {
		return false
	}
	return route.Spec.Security.M2M.ExternalIDP.Client != nil
}

func (route *Route) HasM2MExternalIdpBasic() bool {
	if !route.HasM2M() {
		return false
	}
	if !route.HasM2MExternalIdp() {
		return false
	}
	return route.Spec.Security.M2M.ExternalIDP.Basic != nil
}

type Traffic struct {
	Failover *Failover `json:"failover,omitempty"`
}

type Failover struct {
	TargetZoneName string     `json:"targetZoneName"`
	Upstreams      []Upstream `json:"upstreams"`
	Security       *Security  `json:"security,omitempty"`
}

// RouteStatus defines the observed state of Route
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
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouteSpec   `json:"spec,omitempty"`
	Status RouteStatus `json:"status,omitempty"`
}

var _ types.Object = &Route{}

func (g *Route) GetConditions() []metav1.Condition {
	return g.Status.Conditions
}

func (g *Route) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&g.Status.Conditions, condition)
}

func (g *Route) GetHost() string {
	return g.Spec.Downstreams[0].Host
}

func (g *Route) GetPath() string {
	return g.Spec.Downstreams[0].Path
}

func (g *Route) SetRouteId(id string) {
	g.SetProperty("routeId", id)
}

func (g *Route) SetServiceId(id string) {
	g.SetProperty("serviceId", id)
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
	// If the first upstream has an issuer URL, it is a proxy route
	return len(g.Spec.Upstreams) > 0 && g.Spec.Upstreams[0].IsProxy()
}

func (g *Route) HasFailover() bool {
	return g.Spec.Traffic.Failover != nil
}

func (g *Route) HasFailoverSecurity() bool {
	return g.HasFailover() && g.Spec.Traffic.Failover.Security != nil
}

// IsFailoverSecondary checks if the route is a failover target.
// A Route is a failover target if atleast one failover upstream is a real upstream (not a proxy).
// ! Assumption: It is not possible to mix proxy and non-proxy upstreams in the same failover configuration.
func (g *Route) IsFailoverSecondary() bool {
	if !g.HasFailover() {
		return false
	}
	return slices.ContainsFunc(g.Spec.Traffic.Failover.Upstreams, func(upstream Upstream) bool {
		return !upstream.IsProxy()
	})
}

// +kubebuilder:object:root=true

// RouteList contains a list of Route
type RouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
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
