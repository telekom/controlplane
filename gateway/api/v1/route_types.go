// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strconv"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Upstream struct {
	Url    string `json:"url"`
	Weight int    `json:"weight,omitempty"`
	// TODO remove parts and only keep Url
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
	return len(g.Spec.Upstreams) > 0 && g.Spec.Upstreams[0].IssuerUrl != ""
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
