// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"fmt"
	"sort"
	"sync"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

type snapshotWriter interface {
	setSnapshotFor(context.Context, string, ResourceBundle) error
	clearSnapshotFor(string)
}

// GatewaySnapshotCoordinator owns the complete rendered state for each
// Gateway and serializes publication so concurrent route reconciles cannot
// overwrite one another.
type GatewaySnapshotCoordinator struct {
	mu            sync.Mutex
	writer        snapshotWriter
	contributions map[string]map[string]ResourceBundle
}

var _ XdsClient = &GatewaySnapshotCoordinator{}

func NewGatewaySnapshotCoordinator(writer snapshotWriter) XdsClient {
	return &GatewaySnapshotCoordinator{
		writer:        writer,
		contributions: make(map[string]map[string]ResourceBundle),
	}
}

// RouteIdentity is stable and namespace-safe for aggregate snapshot ownership.
func RouteIdentity(route *gatewayv1.Route) string {
	if route == nil {
		return ""
	}
	return route.Namespace + "/" + route.Name
}

func (c *GatewaySnapshotCoordinator) PublishRoute(ctx context.Context, gateway *gatewayv1.Gateway, routeID string, expectedRouteIDs []string, bundle ResourceBundle) error {
	nodeID, err := NodeIDForGateway(gateway)
	if err != nil {
		return err
	}
	if routeID == "" {
		return fmt.Errorf("route identity is empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	byRoute := c.contributions[nodeID]
	if byRoute == nil {
		byRoute = make(map[string]ResourceBundle)
		c.contributions[nodeID] = byRoute
	}
	previous, existed := byRoute[routeID]
	byRoute[routeID] = bundle
	expected := make(map[string]struct{}, len(expectedRouteIDs))
	for _, id := range expectedRouteIDs {
		expected[id] = struct{}{}
	}
	for id := range byRoute {
		if _, keep := expected[id]; !keep {
			delete(byRoute, id)
		}
	}
	for id := range expected {
		if _, ready := byRoute[id]; !ready {
			return nil
		}
	}
	aggregate, err := aggregateBundles(byRoute)
	if err != nil {
		if existed {
			byRoute[routeID] = previous
		} else {
			delete(byRoute, routeID)
		}
		return fmt.Errorf("aggregating gateway snapshot: %w", err)
	}
	if err := c.writer.setSnapshotFor(ctx, nodeID, aggregate); err != nil {
		if existed {
			byRoute[routeID] = previous
		} else {
			delete(byRoute, routeID)
		}
		return err
	}
	return nil
}

func (c *GatewaySnapshotCoordinator) DeleteRoute(ctx context.Context, gateway *gatewayv1.Gateway, routeID string) error {
	return c.DeleteRouteWithExpected(ctx, gateway, routeID, nil)
}

func (c *GatewaySnapshotCoordinator) DeleteRouteWithExpected(ctx context.Context, gateway *gatewayv1.Gateway, routeID string, expectedRouteIDs []string) error {
	nodeID, err := NodeIDForGateway(gateway)
	if err != nil {
		return err
	}
	if routeID == "" {
		return fmt.Errorf("route identity is empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	byRoute := c.contributions[nodeID]
	previous, existed := byRoute[routeID]
	delete(byRoute, routeID)
	for _, id := range expectedRouteIDs {
		if _, ready := byRoute[id]; !ready {
			if existed {
				byRoute[routeID] = previous
			}
			return nil
		}
	}
	aggregate, err := aggregateBundles(byRoute)
	if err != nil {
		return fmt.Errorf("aggregating gateway snapshot: %w", err)
	}
	if err := c.writer.setSnapshotFor(ctx, nodeID, aggregate); err != nil {
		if existed {
			byRoute[routeID] = previous
		}
		return err
	}
	return nil
}

func (c *GatewaySnapshotCoordinator) ClearGateway(_ context.Context, gateway *gatewayv1.Gateway) error {
	nodeID, err := NodeIDForGateway(gateway)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.writer.setSnapshotFor(context.Background(), nodeID, ResourceBundle{}); err != nil {
		return err
	}
	delete(c.contributions, nodeID)
	return nil
}

func aggregateBundles(byRoute map[string]ResourceBundle) (ResourceBundle, error) {
	routeIDs := make([]string, 0, len(byRoute))
	for id := range byRoute {
		routeIDs = append(routeIDs, id)
	}
	sort.Strings(routeIDs)

	clusters := make(map[string]*clusterv3.Cluster)
	routes := make(map[string]*routev3.RouteConfiguration)
	vhosts := make([]*routev3.VirtualHost, 0, len(routeIDs))
	var template *listenerv3.Listener
	for _, id := range routeIDs {
		bundle := byRoute[id]
		if len(bundle.Listeners) != 1 {
			return ResourceBundle{}, fmt.Errorf("route %q rendered %d listeners, expected one", id, len(bundle.Listeners))
		}
		listener := bundle.Listeners[0]
		if template == nil {
			template = listener
		} else if err := compatibleListeners(template, listener); err != nil {
			return ResourceBundle{}, fmt.Errorf("route %q has unsupported filter configuration: %w", id, err)
		}
		config, err := inlineRouteConfig(listener)
		if err != nil {
			return ResourceBundle{}, fmt.Errorf("route %q: %w", id, err)
		}
		vhosts = append(vhosts, config.GetVirtualHosts()...)
		for _, cluster := range bundle.Clusters {
			if existing := clusters[cluster.GetName()]; existing != nil && !proto.Equal(existing, cluster) {
				return ResourceBundle{}, fmt.Errorf("cluster %q has conflicting definitions", cluster.GetName())
			}
			clusters[cluster.GetName()] = cluster
		}
		for _, route := range bundle.Routes {
			if existing := routes[route.GetName()]; existing != nil && !proto.Equal(existing, route) {
				return ResourceBundle{}, fmt.Errorf("route configuration %q has conflicting definitions", route.GetName())
			}
			routes[route.GetName()] = route
		}
	}

	result := ResourceBundle{
		Clusters: make([]*clusterv3.Cluster, 0, len(clusters)),
		Routes:   make([]*routev3.RouteConfiguration, 0, len(routes)),
	}
	clusterNames := make([]string, 0, len(clusters))
	for name := range clusters {
		clusterNames = append(clusterNames, name)
	}
	sort.Strings(clusterNames)
	for _, name := range clusterNames {
		result.Clusters = append(result.Clusters, clusters[name])
	}
	routeNames := make([]string, 0, len(routes))
	for name := range routes {
		routeNames = append(routeNames, name)
	}
	sort.Strings(routeNames)
	for _, name := range routeNames {
		result.Routes = append(result.Routes, routes[name])
	}
	if template != nil {
		listener, err := listenerWithVirtualHosts(template, vhosts)
		if err != nil {
			return ResourceBundle{}, err
		}
		result.Listeners = []*listenerv3.Listener{listener}
	}
	return result, nil
}

func compatibleListeners(a, b *listenerv3.Listener) error {
	aCopy := proto.Clone(a).(*listenerv3.Listener)
	bCopy := proto.Clone(b).(*listenerv3.Listener)
	if err := clearInlineVirtualHosts(aCopy); err != nil {
		return err
	}
	if err := clearInlineVirtualHosts(bCopy); err != nil {
		return err
	}
	aCopy.Name = ""
	bCopy.Name = ""
	if !proto.Equal(aCopy, bCopy) {
		return fmt.Errorf("HTTP filter chains differ between routes")
	}
	return nil
}

func listenerWithVirtualHosts(template *listenerv3.Listener, vhosts []*routev3.VirtualHost) (*listenerv3.Listener, error) {
	listener := proto.Clone(template).(*listenerv3.Listener)
	listener.Name = "gateway-listener"
	return listener, updateInlineRouteConfig(listener, func(config *routev3.RouteConfiguration) {
		config.Name = "gateway-routes"
		config.VirtualHosts = vhosts
	})
}

func clearInlineVirtualHosts(listener *listenerv3.Listener) error {
	return updateInlineRouteConfig(listener, func(config *routev3.RouteConfiguration) {
		config.Name = ""
		config.VirtualHosts = nil
	})
}

func inlineRouteConfig(listener *listenerv3.Listener) (*routev3.RouteConfiguration, error) {
	manager, _, err := connectionManager(listener)
	if err != nil {
		return nil, err
	}
	config := manager.GetRouteConfig()
	if config == nil {
		return nil, fmt.Errorf("listener does not contain an inline route configuration")
	}
	return config, nil
}

func updateInlineRouteConfig(listener *listenerv3.Listener, update func(*routev3.RouteConfiguration)) error {
	manager, filter, err := connectionManager(listener)
	if err != nil {
		return err
	}
	config := manager.GetRouteConfig()
	if config == nil {
		return fmt.Errorf("listener does not contain an inline route configuration")
	}
	update(config)
	typed, err := anypb.New(manager)
	if err != nil {
		return fmt.Errorf("marshalling aggregated HCM config: %w", err)
	}
	filter.ConfigType = &listenerv3.Filter_TypedConfig{TypedConfig: typed}
	return nil
}

func connectionManager(listener *listenerv3.Listener) (*hcmv3.HttpConnectionManager, *listenerv3.Filter, error) {
	if listener == nil || len(listener.GetFilterChains()) != 1 || len(listener.GetFilterChains()[0].GetFilters()) != 1 {
		return nil, nil, fmt.Errorf("listener must contain exactly one filter chain and one network filter")
	}
	filter := listener.GetFilterChains()[0].GetFilters()[0]
	if filter.GetName() != filterHCM || filter.GetTypedConfig() == nil {
		return nil, nil, fmt.Errorf("listener does not contain an HCM filter")
	}
	manager := &hcmv3.HttpConnectionManager{}
	if err := filter.GetTypedConfig().UnmarshalTo(manager); err != nil {
		return nil, nil, fmt.Errorf("unmarshalling HCM config: %w", err)
	}
	return manager, filter, nil
}
