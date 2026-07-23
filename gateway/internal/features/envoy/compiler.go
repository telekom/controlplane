// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"fmt"
	"sort"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	httprbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// GatewayAggregate is the complete supported Kubernetes input for one Gateway.
type GatewayAggregate struct {
	Environment   string
	Gateway       *gatewayv1.Gateway
	Routes        []gatewayv1.Route
	Consumers     []gatewayv1.Consumer
	ConsumeRoutes []gatewayv1.ConsumeRoute
}

// CompileGateway renders one deterministic complete bundle for a Gateway aggregate.
func CompileGateway(ctx context.Context, aggregate *GatewayAggregate) (ResourceBundle, error) {
	if aggregate.Gateway == nil {
		return ResourceBundle{}, fmt.Errorf("gateway is not set")
	}
	ctx = contextutil.WithEnv(ctx, aggregate.Environment)
	routes := append([]gatewayv1.Route(nil), aggregate.Routes...)
	sort.Slice(routes, func(i, j int) bool {
		return resourceName(routes[i].Namespace, routes[i].Name) < resourceName(routes[j].Namespace, routes[j].Name)
	})

	allAccess := map[string]accessControlIntent{}
	clusters := map[string]*clusterv3.Cluster{}
	endpoints := make([]*endpointv3.ClusterLoadAssignment, 0, len(routes))
	virtualHosts := make([]*routev3.VirtualHost, 0, len(routes))
	domains := make(map[string]string)
	lmsEnabled := false

	for i := range routes {
		route := &routes[i]
		if !route.DeletionTimestamp.IsZero() || !referenceMatches(route.Spec.GatewayRef, aggregate.Gateway) {
			continue
		}
		if len(route.Spec.Backend.Upstreams) == 0 {
			return ResourceBundle{}, fmt.Errorf("route %s/%s has no upstream", route.Namespace, route.Name)
		}
		builder, ok := NewFeatureBuilder(nil, route, nil, aggregate.Gateway).(*Builder)
		if !ok {
			return ResourceBundle{}, fmt.Errorf("envoy feature builder has unexpected type")
		}
		builder.EnableFeature(InstanceAccessControlFeature)
		builder.EnableFeature(InstanceLastMileSecurityFeature)
		builder.SetUpstream(route.Spec.Backend.Upstreams[0])
		for j := range aggregate.ConsumeRoutes {
			consumeRoute := &aggregate.ConsumeRoutes[j]
			if referenceMatches(consumeRoute.Spec.Route, route) {
				builder.AddAllowedConsumers(consumeRoute)
			}
		}
		render, err := builder.renderRoute(ctx)
		if err != nil {
			return ResourceBundle{}, fmt.Errorf("rendering route %s/%s: %w", route.Namespace, route.Name, err)
		}
		for _, domain := range render.vhost.Domains {
			if existing, found := domains[domain]; found {
				return ResourceBundle{}, fmt.Errorf(
					"routes %s and %s have conflicting domain %q", existing, render.name, domain)
			}
			domains[domain] = render.name
		}
		virtualHosts = append(virtualHosts, render.vhost)
		clusters[render.cluster.Name] = render.cluster
		endpoints = append(endpoints, render.endpoint)
		for _, cluster := range render.clusters {
			clusters[cluster.Name] = cluster
		}
		if len(render.access.trustedIssuers) > 0 {
			allAccess[render.name] = render.access
		}
		lmsEnabled = lmsEnabled || render.lms.enabled
	}
	if len(virtualHosts) == 0 {
		virtualHosts = []*routev3.VirtualHost{{
			Name:    resourceName(aggregate.Gateway.Namespace, aggregate.Gateway.Name) + "-not-found",
			Domains: []string{"*"},
			Routes: []*routev3.Route{{
				Match:  &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/"}},
				Action: &routev3.Route_DirectResponse{DirectResponse: &routev3.DirectResponseAction{Status: 404}},
			}},
		}}
	}

	filters, err := aggregateFilters(allAccess, lmsEnabled)
	if err != nil {
		return ResourceBundle{}, err
	}
	baseName := resourceName(aggregate.Gateway.Namespace, aggregate.Gateway.Name)
	routeConfigName := baseName + "-routes"
	listener, err := buildListener(baseName+"-listener", routeConfigName, filters)
	if err != nil {
		return ResourceBundle{}, err
	}
	bundle := ResourceBundle{
		Target: TargetIdentity{
			Environment: aggregate.Environment, Namespace: aggregate.Gateway.Namespace,
			Name: aggregate.Gateway.Name, UID: aggregate.Gateway.UID,
		},
		Source:    SourceMetadata{Resources: aggregateSourceReferences(aggregate)},
		Listeners: []*listenerv3.Listener{listener},
		Routes:    []*routev3.RouteConfiguration{buildRouteConfiguration(routeConfigName, virtualHosts)},
		Endpoints: endpoints,
	}
	for _, cluster := range clusters {
		bundle.Clusters = append(bundle.Clusters, cluster)
	}
	bundle.Sort()
	return bundle, nil
}

func aggregateFilters(access map[string]accessControlIntent, lmsEnabled bool) ([]*hcmv3.HttpFilter, error) {
	intent := accessControlIntent{}
	if len(access) > 0 {
		intent.trustedIssuers = []string{"placeholder"}
		intent.accessControl = true
	}
	filters, err := buildFilters(intent, lmsIntent{enabled: lmsEnabled})
	if err != nil {
		return nil, err
	}
	if jwt := filterByCanonicalName(filters, filterJwtAuthn); jwt != nil {
		typed, err := anyForMessage(buildAggregateJwtAuthn(access))
		if err != nil {
			return nil, err
		}
		jwt.ConfigType = &hcmv3.HttpFilter_TypedConfig{TypedConfig: typed}
	}
	if rbac := filterByCanonicalName(filters, filterRBAC); rbac != nil {
		typed, err := anyForMessage(&httprbacv3.RBAC{})
		if err != nil {
			return nil, err
		}
		rbac.ConfigType = &hcmv3.HttpFilter_TypedConfig{TypedConfig: typed}
	}
	return filters, nil
}

func filterByCanonicalName(filters []*hcmv3.HttpFilter, name string) *hcmv3.HttpFilter {
	for _, filter := range filters {
		if filter.Name == name {
			return filter
		}
	}
	return nil
}

func anyForMessage(message proto.Message) (*anypb.Any, error) {
	typed, err := anypb.New(message)
	if err != nil {
		return nil, fmt.Errorf("marshalling aggregate filter config: %w", err)
	}
	value, err := (proto.MarshalOptions{Deterministic: true}).Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("deterministically marshalling aggregate filter config: %w", err)
	}
	typed.Value = value
	return typed, nil
}

func aggregateSourceReferences(aggregate *GatewayAggregate) []SourceReference {
	references := []SourceReference{sourceReference("Gateway", aggregate.Gateway)}
	for i := range aggregate.Routes {
		if aggregate.Routes[i].DeletionTimestamp.IsZero() &&
			referenceMatches(aggregate.Routes[i].Spec.GatewayRef, aggregate.Gateway) {
			references = append(references, sourceReference("Route", &aggregate.Routes[i]))
		}
	}
	for i := range aggregate.Consumers {
		if aggregate.Consumers[i].DeletionTimestamp.IsZero() &&
			referenceMatches(aggregate.Consumers[i].Spec.Gateway, aggregate.Gateway) {
			references = append(references, sourceReference("Consumer", &aggregate.Consumers[i]))
		}
	}
	for i := range aggregate.ConsumeRoutes {
		if !aggregate.ConsumeRoutes[i].DeletionTimestamp.IsZero() {
			continue
		}
		for j := range aggregate.Routes {
			if aggregate.Routes[j].DeletionTimestamp.IsZero() &&
				referenceMatches(aggregate.Routes[j].Spec.GatewayRef, aggregate.Gateway) &&
				referenceMatches(aggregate.ConsumeRoutes[i].Spec.Route, &aggregate.Routes[j]) {
				references = append(references, sourceReference("ConsumeRoute", &aggregate.ConsumeRoutes[i]))
				break
			}
		}
	}
	return references
}

func referenceMatches(reference types.ObjectRef, object metav1.Object) bool {
	return reference.Equals(object) && (reference.UID == "" || reference.UID == object.GetUID())
}
