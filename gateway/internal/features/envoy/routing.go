// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"fmt"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// listenPort is the downstream port the generated HTTP listener binds to.
// ponytail: fixed port for the POC; the Gateway CRD has no listen-port field.
// Add a spec field / per-gateway config when multiple listeners are needed.
const listenPort uint32 = 10000

// routerFilterName / hcmFilterName are the canonical Envoy filter names.
const (
	hcmFilterName    = "envoy.filters.network.http_connection_manager"
	routerFilterName = "envoy.filters.http.router"
	tlsTransportName = "envoy.transport_sockets.tls"
)

// renderCoreRouting turns a Route and its resolved upstream into the core-routing
// xDS resources: one Listener (HCM + RDS via ADS), one RouteConfiguration
// (VirtualHost with one Route per path), and one Cluster (STRICT_DNS to the
// upstream target). This is the backend-agnostic equivalent of Kong's
// CreateOrReplaceRoute (Service + Route) — see kong/builder.go:243.
func renderCoreRouting(route *gatewayv1.Route, upstream gatewayv1.Upstream, httpFilters []*hcmv3.HttpFilter) (ResourceBundle, error) {
	name := route.Name
	routeConfigName := name + "-routes"
	clusterName := name + "-cluster"

	listener, err := buildListener(name, routeConfigName, httpFilters)
	if err != nil {
		return ResourceBundle{}, fmt.Errorf("building listener: %w", err)
	}

	return ResourceBundle{
		Listeners: []*listenerv3.Listener{listener},
		Routes:    []*routev3.RouteConfiguration{buildRouteConfig(routeConfigName, route.GetHostnames(), route.GetPaths(), clusterName)},
		Clusters:  []*clusterv3.Cluster{buildCluster(clusterName, upstream)},
	}, nil
}

// buildListener creates an HTTP listener whose connection manager resolves its
// routes over RDS via ADS (same snapshot cache). Feature-contributed httpFilters
// are inserted in order before the terminal router filter (router must be last).
func buildListener(name, routeConfigName string, extraFilters []*hcmv3.HttpFilter) (*listenerv3.Listener, error) {
	router, err := anypb.New(&routerv3.Router{})
	if err != nil {
		return nil, fmt.Errorf("marshaling router filter: %w", err)
	}

	httpFilters := make([]*hcmv3.HttpFilter, 0, len(extraFilters)+1)
	httpFilters = append(httpFilters, extraFilters...)
	httpFilters = append(httpFilters, &hcmv3.HttpFilter{
		Name:       routerFilterName,
		ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: router},
	})

	hcm := &hcmv3.HttpConnectionManager{
		CodecType:  hcmv3.HttpConnectionManager_AUTO,
		StatPrefix: name,
		RouteSpecifier: &hcmv3.HttpConnectionManager_Rds{
			Rds: &hcmv3.Rds{
				RouteConfigName: routeConfigName,
				ConfigSource: &corev3.ConfigSource{
					ResourceApiVersion:    corev3.ApiVersion_V3,
					ConfigSourceSpecifier: &corev3.ConfigSource_Ads{Ads: &corev3.AggregatedConfigSource{}},
				},
			},
		},
		HttpFilters: httpFilters,
	}
	hcmAny, err := anypb.New(hcm)
	if err != nil {
		return nil, fmt.Errorf("marshaling http_connection_manager: %w", err)
	}

	return &listenerv3.Listener{
		Name: name,
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "0.0.0.0",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: listenPort},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{{
				Name:       hcmFilterName,
				ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: hcmAny},
			}},
		}},
	}, nil
}

// buildRouteConfig maps hostnames -> VirtualHost.domains (empty = ["*"]) and each
// path prefix -> one Envoy Route to the cluster. Empty paths = prefix "/".
func buildRouteConfig(name string, hostnames, paths []string, clusterName string) *routev3.RouteConfiguration {
	domains := hostnames
	if len(domains) == 0 {
		domains = []string{"*"}
	}
	if len(paths) == 0 {
		paths = []string{"/"}
	}

	routes := make([]*routev3.Route, 0, len(paths))
	for _, prefix := range paths {
		routes = append(routes, &routev3.Route{
			Match: &routev3.RouteMatch{
				PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: prefix},
			},
			Action: &routev3.Route_Route{Route: &routev3.RouteAction{
				ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: clusterName},
			}},
		})
	}

	return &routev3.RouteConfiguration{
		Name: name,
		VirtualHosts: []*routev3.VirtualHost{{
			Name:    name + "-vh",
			Domains: domains,
			Routes:  routes,
		}},
	}
}

// buildCluster creates a STRICT_DNS cluster with the single upstream target as an
// inline endpoint. https upstreams get an UpstreamTlsContext (SNI = hostname).
// ponytail: inline endpoint, no EDS, no server-cert validation. Add a validation
// context and EDS when upstream identity/health must be verified.
func buildCluster(name string, upstream gatewayv1.Upstream) *clusterv3.Cluster {
	c := &clusterv3.Cluster{
		Name:                 name,
		ConnectTimeout:       durationpb.New(5 * time.Second),
		ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STRICT_DNS},
		LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
		LoadAssignment: &endpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpointv3.LocalityLbEndpoints{{
				LbEndpoints: []*endpointv3.LbEndpoint{{
					HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
						Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
							SocketAddress: &corev3.SocketAddress{
								Address:       upstream.GetHostname(),
								PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: uint32(upstream.GetPort())},
							},
						}},
					}},
				}},
			}},
		},
	}

	if upstream.GetScheme() == "https" {
		tlsCtx, _ := anypb.New(&tlsv3.UpstreamTlsContext{Sni: upstream.GetHostname()})
		c.TransportSocket = &corev3.TransportSocket{
			Name:       tlsTransportName,
			ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: tlsCtx},
		}
	}

	return c
}
