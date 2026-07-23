// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"fmt"
	"sort"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	setmetadatav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/set_metadata/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	upstreamsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

const (
	filterSetMetadata          = "envoy.filters.http.set_metadata"
	filterRateLimit            = "envoy.filters.http.ratelimit"
	rateLimitDomain            = "tardis-gateway"
	rateLimitCluster           = "ratelimit"
	rateLimitHost              = "ratelimit"
	rateLimitPort              = 8081
	rateLimitMetadataNamespace = "tardis.ratelimit"
	rateLimitConsumerHeader    = "x-tardis-ratelimit-consumer"
)

type RateLimitFeature struct {
	priority int
}

var InstanceRateLimitFeature = &RateLimitFeature{priority: 10}

func (f *RateLimitFeature) Name() gatewayv1.FeatureType { return gatewayv1.FeatureTypeRateLimit }

func (f *RateLimitFeature) Priority() int { return f.priority }

func (f *RateLimitFeature) IsUsed(_ context.Context, builder features.FeatureBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok || route.Spec.PassThrough {
		return false
	}
	if !route.IsProxy() && route.Spec.Traffic.RateLimit != nil {
		return true
	}
	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.Spec.Route.Equals(route) && consumer.HasTrafficRateLimit() {
			return true
		}
	}
	return false
}

func (f *RateLimitFeature) Apply(_ context.Context, builder EnvoyFeatureBuilder) error {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	var routeLimit *gatewayv1.RateLimit
	if !route.IsProxy() {
		routeLimit = route.Spec.Traffic.RateLimit
	}
	consumerLimits := map[string]gatewayv1.Limits{}
	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.Spec.Route.Equals(route) && consumer.HasTrafficRateLimit() {
			consumerLimits[consumer.Spec.ConsumerName] = consumer.Spec.Traffic.RateLimit.Limits
		}
	}
	if len(consumerLimits) > 0 && len(route.GetTrustedIssuers()) == 0 {
		return fmt.Errorf("consumer rate limits require a trusted JWT issuer")
	}
	builder.ConfigureRateLimit(fmt.Sprintf("%s/%s", route.Namespace, route.Name), routeLimit, consumerLimits)
	return nil
}

type rateLimitIntent struct {
	enabled           bool
	routeID           string
	routeLimits       gatewayv1.Limits
	consumerLimits    map[string]gatewayv1.Limits
	faultTolerant     bool
	hideClientHeaders bool
}

func newRateLimitIntent(routeID string, routeLimit *gatewayv1.RateLimit, consumerLimits map[string]gatewayv1.Limits) rateLimitIntent {
	intent := rateLimitIntent{
		enabled:        routeLimit != nil || len(consumerLimits) > 0,
		routeID:        routeID,
		consumerLimits: consumerLimits,
		faultTolerant:  true,
	}
	if routeLimit != nil {
		intent.routeLimits = routeLimit.Limits
		intent.faultTolerant = routeLimit.Options.FaultTolerant
		intent.hideClientHeaders = routeLimit.Options.HideClientHeaders
	}
	return intent
}

type rateLimitWindow struct {
	name  string
	unit  string
	limit int
}

func rateLimitWindows(limits gatewayv1.Limits) []rateLimitWindow {
	return []rateLimitWindow{
		{name: "second", unit: "SECOND", limit: limits.Second},
		{name: "minute", unit: "MINUTE", limit: limits.Minute},
		{name: "hour", unit: "HOUR", limit: limits.Hour},
	}
}

func buildRateLimitMetadataFilter(intent rateLimitIntent) (*hcmv3.HttpFilter, error) {
	routeValues := map[string]any{}
	for _, window := range rateLimitWindows(intent.routeLimits) {
		if window.limit > 0 {
			routeValues[window.name] = limitOverrideValue(window)
		}
	}
	consumerValues := map[string]any{}
	for consumer, limits := range intent.consumerLimits {
		windows := map[string]any{}
		for _, window := range rateLimitWindows(limits) {
			if window.limit > 0 {
				windows[window.name] = limitOverrideValue(window)
			}
		}
		consumerValues[consumer] = windows
	}
	value, err := structpb.NewStruct(map[string]any{"route": routeValues, "consumer": consumerValues})
	if err != nil {
		return nil, fmt.Errorf("building rate-limit metadata: %w", err)
	}
	return mkFilter(filterSetMetadata, &setmetadatav3.Config{Metadata: []*setmetadatav3.Metadata{{
		MetadataNamespace: rateLimitMetadataNamespace,
		AllowOverwrite:    true,
		Value:             value,
	}}})
}

func limitOverrideValue(window rateLimitWindow) map[string]any {
	return map[string]any{"requests_per_unit": window.limit, "unit": window.unit}
}

func buildRateLimitFilter(intent rateLimitIntent) *ratelimitfilterv3.RateLimit {
	return &ratelimitfilterv3.RateLimit{
		Domain:                         rateLimitDomain,
		Timeout:                        durationpb.New(50 * time.Millisecond),
		FailureModeDeny:                !intent.faultTolerant,
		DisableXEnvoyRatelimitedHeader: intent.hideClientHeaders,
		RateLimitService: &ratelimitconfigv3.RateLimitServiceConfig{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: rateLimitCluster},
				},
			},
			TransportApiVersion: corev3.ApiVersion_V3,
		},
	}
}

func buildRateLimitDescriptors(intent rateLimitIntent) []*routev3.RateLimit {
	if !intent.enabled {
		return nil
	}
	descriptors := make([]*routev3.RateLimit, 0)
	for _, window := range rateLimitWindows(intent.routeLimits) {
		if window.limit > 0 {
			descriptors = append(descriptors, rateLimitDescriptor(intent.routeID, "", window))
		}
	}
	consumers := make([]string, 0, len(intent.consumerLimits))
	for consumer := range intent.consumerLimits {
		consumers = append(consumers, consumer)
	}
	sort.Strings(consumers)
	for _, consumer := range consumers {
		for _, window := range rateLimitWindows(intent.consumerLimits[consumer]) {
			if window.limit > 0 {
				descriptors = append(descriptors, rateLimitDescriptor(intent.routeID, consumer, window))
			}
		}
	}
	return descriptors
}

func rateLimitDescriptor(routeID, consumer string, window rateLimitWindow) *routev3.RateLimit {
	actions := []*routev3.RateLimit_Action{genericKeyAction("route", routeID)}
	metadataPath := []string{"route", window.name}
	if consumer != "" {
		actions = append(actions, consumerAction(consumer))
		metadataPath = []string{"consumer", consumer, window.name}
	}
	actions = append(actions, genericKeyAction("window", window.name))
	return &routev3.RateLimit{
		Actions: actions,
		Limit: &routev3.RateLimit_Override{OverrideSpecifier: &routev3.RateLimit_Override_DynamicMetadata_{
			DynamicMetadata: &routev3.RateLimit_Override_DynamicMetadata{MetadataKey: metadataKey(metadataPath...)},
		}},
	}
}

func genericKeyAction(key, value string) *routev3.RateLimit_Action {
	return &routev3.RateLimit_Action{ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
		GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorKey: key, DescriptorValue: value},
	}}
}

func consumerAction(consumer string) *routev3.RateLimit_Action {
	return &routev3.RateLimit_Action{ActionSpecifier: &routev3.RateLimit_Action_HeaderValueMatch_{
		HeaderValueMatch: &routev3.RateLimit_Action_HeaderValueMatch{
			DescriptorKey:   "consumer",
			DescriptorValue: consumer,
			ExpectMatch:     wrapperspb.Bool(true),
			Headers: []*routev3.HeaderMatcher{{
				Name: rateLimitConsumerHeader,
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Exact{Exact: consumer},
				}},
			}},
		},
	}}
}

func metadataKey(path ...string) *metadatav3.MetadataKey {
	segments := make([]*metadatav3.MetadataKey_PathSegment, 0, len(path))
	for _, key := range path {
		segments = append(segments, &metadatav3.MetadataKey_PathSegment{
			Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: key},
		})
	}
	return &metadatav3.MetadataKey{Key: rateLimitMetadataNamespace, Path: segments}
}

func buildRateLimitCluster() (*clusterv3.Cluster, error) {
	cluster := buildCluster(rateLimitCluster, rateLimitHost, rateLimitPort)
	http2Options, err := anypb.New(&upstreamsv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &corev3.Http2ProtocolOptions{},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling rate-limit HTTP/2 options: %w", err)
	}
	cluster.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": http2Options,
	}
	return cluster, nil
}
