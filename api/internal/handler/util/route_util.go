// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"net/url"
	"slices"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

const (
	// GatewayConsumerName is the name of the gateway mesh-client that is used to proxy requests across zones.
	// It must be added to DefaultConsumers on routes that are the target of a cross-zone proxy.
	GatewayConsumerName = "gateway"
)

const labelTrue = "true"

var LabelFailoverSecondary = config.BuildLabelKey("failover.secondary")

type CreateRouteOptions struct {
	FailoverUpstreams   []apiapi.Upstream
	FailoverZones       []types.ObjectRef
	FailoverSecurity    *apiapi.Security
	ReturnReferenceOnly bool // If true, the route will not be created, but only the reference will be returned.

	// Rate limit configuration for the consumer
	ServiceRateLimit *apiapi.RateLimitConfig

	// In case of proxy route, consumer rate limits must be set as well
	ConsumerRateLimit *apiapi.Limits

	// IsProxyTarget indicates that this route is the target of a cross-zone proxy route.
	// When true, the gateway mesh-client consumer is added to the route's DefaultConsumers
	// to allow the proxy route to access this route.
	IsProxyTarget bool

	// TrustedIssuers is the list of trusted token issuers for this route.
	// For real routes: includes the zone's IDP issuer (for consumer access) and
	// LMS issuers from proxy zones (for cross-zone mesh access).
	// For proxy routes: includes the subscriber zone's IDP issuer (for consumer access).
	TrustedIssuers []string

	// RealmName is the identity realm name used by the Jumper sidecar for
	// Last-Mile-Security token issuance. Typically equals the environment name.
	RealmName string

	// AdditionalHostnames are used to add extra hostnames to the route's Hostnames list.
	// This is useful for failover routes that need to accept traffic from multiple hostnames.
	AdditionalHostnames []string
	// AdditionalPaths are used to add extra paths to the route's Paths list.
	AdditionalPaths []string

	// ResolvedClaims holds the exposure's M2M claims after static ValueFrom sources
	// (ProviderClientId, BasePath) have been resolved to literals in the handler.
	// When set, it overrides the claims mapped from the exposure's own security spec.
	ResolvedClaims *apiapi.Claims
}

type CreateRouteOption func(*CreateRouteOptions)

type CreateConsumeRouteOptions struct {
	ConsumerRateLimit *apiapi.Limits // Rate limit configuration for the consumer

	FailoverFlag bool // If true, this consume route is created due to failover
}

func (o *CreateConsumeRouteOptions) HasConsumerRateLimit() bool { return o.ConsumerRateLimit != nil }

type CreateConsumeRouteOption func(*CreateConsumeRouteOptions)

// WithFailoverUpstreams sets the failover upstreams for the route.
// A Proxy-Route created using CreateProxyRoute with this option will have the failover upstreams set
// and automatically be a failover secondary route.
func WithFailoverUpstreams(failoverUpstreams ...apiapi.Upstream) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.FailoverUpstreams = failoverUpstreams
	}
}

// WithFailoverZones sets the failover zones for the route.
// A Proxy-Route created using CreateProxyRoute with this option will have failover targets set.
// This will result in a Proxy-Route that will proxy requests to the failover zones (secondary routes).
func WithFailoverZones(failoverZones []types.ObjectRef) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.FailoverZones = failoverZones
	}
}

// WithResolvedClaims sets the exposure's M2M claims after static ValueFrom sources
// (ProviderClientId, BasePath) have been resolved to literals in the handler.
func WithResolvedClaims(claims *apiapi.Claims) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.ResolvedClaims = claims
	}
}

// WithFailoverSecurity sets the failover security for the route.
// A Proxy-Route created using CreateProxyRoute with this option will have the failover security set.
// Only applicable if IsFailoverSecondary() is true.
func WithFailoverSecurity(security *apiapi.Security) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.FailoverSecurity = security
	}
}

func WithAdditionalHostnames(hostnames ...string) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.AdditionalHostnames = append(opts.AdditionalHostnames, hostnames...)
	}
}

func WithAdditionalPaths(paths ...string) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.AdditionalPaths = append(opts.AdditionalPaths, paths...)
	}
}

// ReturnReferenceOnly indicates that the route should not be created, but only the reference should be returned.
// This is useful for cases where you only need the route reference, e.g., for cleanup operations or when the route is already created.
func ReturnReferenceOnly() CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.ReturnReferenceOnly = true
	}
}

func WithServiceRateLimit(rateLimit *apiapi.RateLimitConfig) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.ServiceRateLimit = rateLimit
	}
}

func WithConsumerRateLimit(limits *apiapi.Limits) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.ConsumerRateLimit = limits
	}
}

// WithProxyTarget marks the route as a proxy target, indicating that the gateway
// mesh-client consumer should be added to the route's DefaultConsumers.
// This mirrors the Event domain's WithProxyTarget option.
func WithProxyTarget(isProxyTarget bool) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.IsProxyTarget = isProxyTarget
	}
}

// WithTrustedIssuers sets the trusted token issuers for the route.
// These issuers are used by the gateway's JWT plugin to validate incoming tokens.
func WithTrustedIssuers(issuers []string) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.TrustedIssuers = slices.Clone(issuers)
	}
}

// AddTrustedIssuers appends trusted token issuers to the route's existing list of trusted issuers.
func AddTrustedIssuers(issuers ...string) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.TrustedIssuers = append(opts.TrustedIssuers, issuers...)
	}
}

// WithRealmName sets the identity realm name on the route's Security.
// The Jumper sidecar uses this to determine which realm to use for LMS token issuance.
func WithRealmName(realmName string) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.RealmName = realmName
	}
}

// IsFailoverSecondary checks if the route is a failover secondary route.
// This means that this route has the real upstream as a failover upstream.
func (o *CreateRouteOptions) IsFailoverSecondary() bool {
	return len(o.FailoverUpstreams) > 0
}

// HasFailover checks if the route has failover zones configured.
// This means that this route is used as a proxy to the failover zones.
func (o *CreateRouteOptions) HasFailover() bool {
	return len(o.FailoverZones) > 0
}

func (o *CreateRouteOptions) HasServiceRateLimit() bool {
	return o.ServiceRateLimit != nil
}

// WithRateLimit sets the rate limit configuration for the ConsumeRoute
func WithConsumerRouteRateLimit(rateLimit apiapi.Limits) CreateConsumeRouteOption {
	return func(opts *CreateConsumeRouteOptions) {
		opts.ConsumerRateLimit = &rateLimit
	}
}

func WithFailoverLabel() CreateConsumeRouteOption {
	return func(opts *CreateConsumeRouteOptions) {
		opts.FailoverFlag = true
	}
}

func MakeRouteName(apiBasePath string) string {
	return labelutil.NormalizeValue(apiBasePath)
}

func CreateProxyRoute(ctx context.Context, downstreamZoneRef, upstreamZoneRef types.ObjectRef, apiBasePath string, opts ...CreateRouteOption) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	options := &CreateRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Downstream: get zone + default preset to derive hostnames/paths and gateway ref
	downstreamPreset, downstreamZone, err := GetDefaultPresetForZone(ctx, downstreamZoneRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get downstream preset for zone %s", downstreamZoneRef.String())
	}
	if downstreamZone.Status.Gateway == nil {
		return nil, errors.Errorf("zone %s has no gateway reference in status", downstreamZoneRef.String())
	}

	// Upstream: get zone + default preset to derive the upstream URL
	upstreamPreset, upstreamZone, err := GetDefaultPresetForZone(ctx, upstreamZoneRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get upstream preset for zone %s", upstreamZoneRef.Name)
	}

	// Creating the Route
	routeName := MakeRouteName(apiBasePath)

	proxyRoute := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(routeName),
			Namespace: downstreamZone.Status.Namespace,
		},
	}

	if options.ReturnReferenceOnly {
		// Return early with just the reference (name + namespace)
		return proxyRoute, nil
	}

	mutate := func() error {
		proxyRoute.Labels = map[string]string{
			apiapi.BasePathLabelKey:      labelutil.NormalizeLabelValue(apiBasePath),
			config.BuildLabelKey("zone"): labelutil.NormalizeValue(downstreamZone.GetName()),
			config.BuildLabelKey("type"): "proxy",
		}

		// Upstream for proxy route: points at the upstream zone's gateway URL for this basepath
		upstreamUrl, joinErr := url.JoinPath(upstreamPreset.GetDefaultUrl(), apiBasePath)
		if joinErr != nil {
			return errors.Wrap(joinErr, "failed to build upstream URL for proxy route")
		}
		upstream, upstreamErr := AsUpstream(upstreamUrl, 0)
		if upstreamErr != nil {
			return errors.Wrap(upstreamErr, "failed to create upstream")
		}

		hostnames, paths := downstreamPreset.ResolveHostnamesAndPaths(apiBasePath)

		proxyRoute.Spec = gatewayapi.RouteSpec{
			GatewayRef: *downstreamZone.Status.Gateway,
			Type:       gatewayapi.RouteTypeProxy,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Traffic:    gatewayapi.Traffic{},
		}
		proxyRoute.Spec.Hostnames = slices.Concat(hostnames, options.AdditionalHostnames)
		slices.Sort(proxyRoute.Spec.Hostnames)
		proxyRoute.Spec.Hostnames = slices.Compact(slices.Clip(proxyRoute.Spec.Hostnames))

		proxyRoute.Spec.Paths = slices.Concat(paths, options.AdditionalPaths)
		slices.Sort(proxyRoute.Spec.Paths)
		proxyRoute.Spec.Paths = slices.Compact(slices.Clip(proxyRoute.Spec.Paths))

		// Set trusted issuers for consumer token validation on the proxy route.
		// The proxy route lives in the subscriber zone and accepts consumer traffic,
		// so it must validate tokens from the subscriber zone's IDP.
		if downstreamZone.Status.Links.Issuer != "" {
			proxyRoute.Spec.Security.TrustedIssuers = []string{downstreamZone.Status.Links.Issuer}
		}
		// Append any additional trusted issuers from options (e.g. consumer failover IDP issuers)
		// and deduplicate (the downstream issuer may already be in the consumer failover list).
		if len(options.TrustedIssuers) > 0 {
			proxyRoute.Spec.Security.TrustedIssuers = append(
				proxyRoute.Spec.Security.TrustedIssuers, options.TrustedIssuers...)
			slices.Sort(proxyRoute.Spec.Security.TrustedIssuers)
			proxyRoute.Spec.Security.TrustedIssuers = slices.Compact(proxyRoute.Spec.Security.TrustedIssuers)
		}
		if options.RealmName != "" {
			proxyRoute.Spec.Security.RealmName = options.RealmName
		}

		logger.Info("Creating proxy route", "route", proxyRoute.Name, "namespace", proxyRoute.Namespace, "failover", options.HasFailover())

		if options.HasServiceRateLimit() {
			proxyRoute.Spec.Traffic = gatewayapi.Traffic{
				RateLimit: mapProviderRateLimitToGatewayRateLimit(options.ServiceRateLimit),
			}
		}

		if options.IsFailoverSecondary() {
			if secondaryErr := configureAsFailoverTarget(ctx, proxyRoute, options, upstreamZone.Name); secondaryErr != nil {
				return secondaryErr
			}
		}

		if options.HasFailover() {
			if primaryErr := addFailoverFallback(ctx, proxyRoute, options, apiBasePath, upstreamZone.Name); primaryErr != nil {
				return primaryErr
			}
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, proxyRoute, mutate)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create proxy route")
	}

	return proxyRoute, nil
}

// configureAsFailoverTarget configures the proxy route as a failover target (secondary route).
// This route becomes the destination where traffic lands when the primary zone is unavailable.
func configureAsFailoverTarget(_ context.Context, proxyRoute *gatewayapi.Route, options *CreateRouteOptions, upstreamZoneName string) error {
	proxyRoute.Labels[LabelFailoverSecondary] = labelTrue
	proxyRoute.Spec.Type = gatewayapi.RouteTypeSecondary

	// A failover secondary route is the target of cross-zone proxy requests,
	// so the gateway mesh-client must be allowed to access it.
	proxyRoute.Spec.Security.DefaultConsumers = append(proxyRoute.Spec.Security.DefaultConsumers, GatewayConsumerName)

	// The failover secondary also needs the same TrustedIssuers as the primary route,
	// since proxy routes may fail over to it and present LMS tokens.
	if len(options.TrustedIssuers) > 0 {
		proxyRoute.Spec.Security.TrustedIssuers = append(proxyRoute.Spec.Security.TrustedIssuers, options.TrustedIssuers...)
		slices.Sort(proxyRoute.Spec.Security.TrustedIssuers)
		proxyRoute.Spec.Security.TrustedIssuers = slices.Compact(proxyRoute.Spec.Security.TrustedIssuers)
	}

	failoverTargets := make([]gatewayapi.FailoverTarget, 0, len(options.FailoverUpstreams))
	for _, rawUpstream := range options.FailoverUpstreams {
		failoverUpstream, upstreamErr := AsUpstream(rawUpstream.Url, int32(rawUpstream.Weight)) //nolint:gosec // weight is a small positive integer
		if upstreamErr != nil {
			return errors.Wrapf(upstreamErr, "failed to create failover upstream %s", rawUpstream.Url)
		}
		failoverTargets = append(failoverTargets, gatewayapi.FailoverTarget{Upstream: failoverUpstream})
	}

	proxyRoute.Spec.Traffic = gatewayapi.Traffic{
		Failover: &gatewayapi.Failover{
			TargetZoneName: upstreamZoneName,
			Targets:        failoverTargets,
		},
	}

	// Add the provided security config (mostly copied from primary-route)
	// to the failover config of the secondary route
	if options.FailoverSecurity != nil {
		proxyRoute.Spec.Traffic.Failover.Security = mapSecurity(options.FailoverSecurity, options.ResolvedClaims)
	}

	if options.RealmName != "" {
		proxyRoute.Spec.Traffic.Failover.Security.RealmName = options.RealmName
	}

	// Other features like rate limiting, circuit breaking, etc. cannot be added as they would otherwise
	// be applied to the route itself, independent of the failover configuration.

	return nil
}

// addFailoverFallback configures the proxy route with failover targets for when the primary zone is unavailable.
// Each target points to a failover zone's gateway where a secondary route lives.
// The jumper iterates the targets in order and picks the first healthy zone.
func addFailoverFallback(ctx context.Context, proxyRoute *gatewayapi.Route, options *CreateRouteOptions, apiBasePath, upstreamZoneName string) error {
	failoverTargets := make([]gatewayapi.FailoverTarget, 0, len(options.FailoverZones))

	for _, failoverZone := range options.FailoverZones {
		failoverPreset, _, err := GetDefaultPresetForZone(ctx, failoverZone)
		if err != nil {
			return errors.Wrapf(err, "failed to get failover zone %s", failoverZone.String())
		}
		failoverUrl, err := url.JoinPath(failoverPreset.GetDefaultUrl(), apiBasePath)
		if err != nil {
			return errors.Wrapf(err, "failed to build failover URL for zone %s", failoverZone.String())
		}

		failoverUpstream, err := AsUpstream(failoverUrl, 0)
		if err != nil {
			return errors.Wrapf(err, "failed to create failover upstream for zone %s", failoverZone.String())
		}

		failoverTargets = append(failoverTargets, gatewayapi.FailoverTarget{
			ZoneName: failoverZone.Name,
			Upstream: failoverUpstream,
		})
	}

	proxyRoute.Spec.Traffic = gatewayapi.Traffic{
		Failover: &gatewayapi.Failover{
			TargetZoneName: upstreamZoneName,
			Targets:        failoverTargets,
		},
	}
	return nil
}

// CleanupProxyRoute deletes the route only if no other subscriptions (size > 1) for this route exist
func CleanupProxyRoute(ctx context.Context, routeRef *types.ObjectRef, opts ...CreateRouteOption) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	if routeRef == nil {
		return nil
	}
	logger := log.FromContext(ctx).WithValues("route.name", routeRef.Name, "route.namespace", routeRef.Namespace)

	options := &CreateRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	route := &gatewayapi.Route{}
	err := scopedClient.Get(ctx, routeRef.K8s(), route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "failed to get route")
	}

	if route.GetLabels()[config.BuildLabelKey("type")] == "real" { // DO NOT DELETE REAL ROUTES
		logger.V(1).Info("🫷 Not deleting route as it is a real route")
		return nil
	}

	if route.GetLabels()[LabelFailoverSecondary] == labelTrue { // DO NOT DELETE FAILOVER ROUTES
		logger.V(1).Info("🫷 Not deleting route as it is a failover secondary")
		return nil
	}

	basePath := route.GetLabels()[apiapi.BasePathLabelKey]
	zone := route.GetLabels()[config.BuildLabelKey("zone")]

	apiSubscriptions := &apiapi.ApiSubscriptionList{}
	err = scopedClient.List(ctx, apiSubscriptions, client.MatchingLabels{
		apiapi.BasePathLabelKey:      basePath,
		config.BuildLabelKey("zone"): zone,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to list routes matching basePath %s in zone %s", basePath, zone)
	}

	if len(apiSubscriptions.Items) > 1 {
		logger.Info("🫷 Not deleting route as more than 1 subscriptions exists")
		return nil
	}

	logger.Info("🧹 Deleting route as no more subscriptions exist")

	err = scopedClient.Delete(ctx, route)
	if err != nil {
		return errors.Wrapf(err, "failed to delete route")
	}
	logger.Info("✅ Successfully deleted obsolete route")

	return nil
}

// CleanupStaleProxyRoutes uses the JanitorClient's Cleanup() to delete any stale proxy Routes
// for the given apiBasePath that were NOT created/updated in this reconciliation cycle.
// This handles zone changes: when subscriptions move or are deleted, old proxy routes are cleaned up.
// NOTE: This does NOT delete provider failover routes (labeled with failover.secondary=true),
// as those are managed separately by ApiExposure and should not be cleaned up here.
func CleanupStaleProxyRoutes(ctx context.Context, apiBasePath string) (int, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// Use janitor's Cleanup for all proxy routes with this basepath
	// The janitor will only delete routes that were NOT touched in this reconciliation cycle
	deleted, err := c.Cleanup(ctx, &gatewayapi.RouteList{}, []client.ListOption{
		client.MatchingLabels{
			apiapi.BasePathLabelKey:      labelutil.NormalizeLabelValue(apiBasePath),
			config.BuildLabelKey("type"): "proxy",
		},
	})
	if err != nil {
		return deleted, errors.Wrapf(err, "failed to cleanup stale proxy routes for basepath %q", apiBasePath)
	}

	return deleted, nil
}

func CreateRealRoute(ctx context.Context, downstreamZoneRef types.ObjectRef, apiExposure *apiapi.ApiExposure, opts ...CreateRouteOption) (*gatewayapi.Route, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	options := &CreateRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Get zone + default preset to derive hostnames/paths and gateway ref
	preset, zone, err := GetDefaultPresetForZone(ctx, downstreamZoneRef)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to get default preset for zone %s", downstreamZoneRef.String()))
	}
	if zone.Status.Gateway == nil {
		return nil, errors.Errorf("zone %s has no gateway reference in status", downstreamZoneRef.String())
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeRouteName(apiExposure.Spec.ApiBasePath),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			apiapi.BasePathLabelKey:      labelutil.NormalizeLabelValue(apiExposure.Spec.ApiBasePath),
			config.BuildLabelKey("zone"): labelutil.NormalizeValue(zone.Name),
			config.BuildLabelKey("type"): "real",
		}

		gatewayUpstreams := make([]gatewayapi.Upstream, 0, len(apiExposure.Spec.Upstreams))
		for _, upstream := range apiExposure.Spec.Upstreams {
			gatewayUpstream, upstreamErr := AsUpstream(upstream.Url, int32(upstream.Weight)) //nolint:gosec // weight is a small positive integer
			if upstreamErr != nil {
				return errors.Wrapf(upstreamErr, "failed to create upstream for URL %s", upstream.Url)
			}
			gatewayUpstreams = append(gatewayUpstreams, gatewayUpstream)
		}

		hostnames, paths := preset.ResolveHostnamesAndPaths(apiExposure.Spec.ApiBasePath)

		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *zone.Status.Gateway,
			Type:       gatewayapi.RouteTypePrimary,
			Backend:    gatewayapi.Backend{Upstreams: gatewayUpstreams},
			Traffic:    gatewayapi.Traffic{},
		}
		route.Spec.Hostnames = slices.Concat(hostnames, options.AdditionalHostnames)
		route.Spec.Paths = slices.Concat(paths, options.AdditionalPaths)
		slices.Sort(route.Spec.Hostnames)
		slices.Sort(route.Spec.Paths)
		route.Spec.Hostnames = slices.Compact(slices.Clip(route.Spec.Hostnames))
		route.Spec.Paths = slices.Compact(slices.Clip(route.Spec.Paths))

		route.Spec.Transformation = mapTransformation(apiExposure.Spec.Transformation)
		route.Spec.Security = mapSecurity(apiExposure.Spec.Security, options.ResolvedClaims)

		if options.IsProxyTarget {
			// If this Route is the target of a cross-zone proxy Route,
			// the gateway mesh-client must be allowed to access it.
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, GatewayConsumerName)
		}

		if len(options.TrustedIssuers) > 0 {
			slices.Sort(options.TrustedIssuers)
			route.Spec.Security.TrustedIssuers = slices.Compact(slices.Clip(options.TrustedIssuers))
		}
		if options.RealmName != "" {
			route.Spec.Security.RealmName = options.RealmName
		}

		if apiExposure.HasProviderRateLimit() {
			route.Spec.Traffic.RateLimit = mapProviderRateLimitToGatewayRateLimit(apiExposure.Spec.Traffic.RateLimit.Provider)
		}

		// switch from pointer to non-pointer (
		if apiExposure.HasCircuitBreaker() {
			route.Spec.Traffic.CircuitBreaker = mapCircuitBreaker(apiExposure.Spec.Traffic.CircuitBreaker)
		}

		return nil
	}

	_, err = scopedClient.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update route: %s in namespace: %s", route.Name, route.Namespace)
	}

	return route, nil
}

func CreateConsumeRoute(ctx context.Context, apiSub *apiapi.ApiSubscription, downstreamZoneRef, routeRef types.ObjectRef, clientId string, opts ...CreateConsumeRouteOption) (*gatewayapi.ConsumeRoute, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	options := &CreateConsumeRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	name := downstreamZoneRef.Name + "--" + apiSub.GetName()
	routeConsumer := &gatewayapi.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: apiSub.GetNamespace(),
		},
	}

	mutate := func() error {
		if err := controllerutil.SetControllerReference(apiSub, routeConsumer, scopedClient.Scheme()); err != nil {
			return errors.Wrapf(err, "failed to set owner-reference on %v", routeConsumer)
		}
		routeConsumer.Labels = apiSub.GetLabels()

		if options.FailoverFlag {
			routeConsumer.Labels[config.BuildLabelKey("failover")] = "true"
		}

		routeConsumer.Spec = gatewayapi.ConsumeRouteSpec{
			Route:        routeRef,
			ConsumerName: clientId,
			Security:     mapConsumerSecurity(apiSub.Spec.Security),
		}

		if options.HasConsumerRateLimit() {
			if routeConsumer.Spec.Traffic == nil {
				routeConsumer.Spec.Traffic = &gatewayapi.ConsumeRouteTraffic{}
			}
			routeConsumer.Spec.Traffic.RateLimit = &gatewayapi.ConsumeRouteRateLimit{
				Limits: mapLimitsToGatewayLimits(*options.ConsumerRateLimit),
			}
		}

		return nil
	}

	_, err := scopedClient.CreateOrUpdate(ctx, routeConsumer, mutate)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to create ConsumeRoute %s in namespace: %s",
			apiSub.GetName(), apiSub.GetNamespace())
	}

	return routeConsumer, nil
}

// ResolveExposureClaims resolves the static ValueFrom claim sources of an exposure's
// M2M claims into literals: ProviderClientId -> the application's client id,
// BasePath -> the exposure base path. ConsumerClientId stays symbolic for Jumper to
// resolve per-request. A user-provided literal is copied through unchanged. Returns nil
// when there are no M2M claims to resolve.
func ResolveExposureClaims(apiExp *apiapi.ApiExposure, clientId string) *apiapi.Claims {
	if apiExp.Spec.Security == nil || apiExp.Spec.Security.M2M == nil {
		return nil
	}
	claims := apiExp.Spec.Security.M2M.Claims
	if claims == nil || claims.Aud == nil {
		return nil
	}

	aud := claims.Aud
	resolved := &apiapi.Claim{}
	switch {
	case aud.Value != "":
		resolved.Value = aud.Value
	case aud.ValueFrom == apiapi.ClaimValueFromProviderClientId:
		resolved.Value = clientId
	case aud.ValueFrom == apiapi.ClaimValueFromBasePath:
		resolved.Value = apiExp.Spec.ApiBasePath
	case aud.ValueFrom == apiapi.ClaimValueFromConsumerClientId:
		resolved.ValueFrom = apiapi.ClaimValueFromConsumerClientId
	default:
		return nil
	}

	return &apiapi.Claims{Aud: resolved}
}

func mapSecurity(apiSecurity *apiapi.Security, resolvedClaims *apiapi.Claims) gatewayapi.Security {
	if apiSecurity == nil {
		return gatewayapi.Security{}
	}

	security := gatewayapi.Security{}

	if apiSecurity.M2M != nil {
		security.M2M = &gatewayapi.Machine2MachineAuthentication{
			Scopes: apiSecurity.M2M.Scopes,
		}
		security.M2M.ExternalIDP = mapExternalIDP(apiSecurity.M2M.ExternalIDP)
		if apiSecurity.M2M.Basic != nil {
			security.M2M.Basic = &gatewayapi.BasicAuthCredentials{
				Username: apiSecurity.M2M.Basic.Username,
				Password: apiSecurity.M2M.Basic.Password,
			}
		}
		claims := apiSecurity.M2M.Claims
		if resolvedClaims != nil {
			claims = resolvedClaims
		}
		security.M2M.Claims = mapClaims(claims)
	}

	return security
}

// mapClaims flattens the api Claims (currently only aud) into the gateway's flat
// []Claim list. Value is the CP-resolved literal; ValueFrom stays symbolic.
func mapClaims(apiClaims *apiapi.Claims) []gatewayapi.Claim {
	if apiClaims == nil || apiClaims.Aud == nil {
		return nil
	}
	return []gatewayapi.Claim{{
		Key:       "aud",
		Value:     apiClaims.Aud.Value,
		ValueFrom: gatewayapi.ClaimValueFrom(apiClaims.Aud.ValueFrom),
	}}
}

func mapExternalIDP(externalIDP *apiapi.ExternalIdentityProvider) *gatewayapi.ExternalIdentityProvider {
	if externalIDP == nil {
		return nil
	}

	idp := &gatewayapi.ExternalIdentityProvider{
		TokenEndpoint: externalIDP.TokenEndpoint,
		TokenRequest:  gatewayapi.TokenRequestMethod(externalIDP.TokenRequest),
		GrantType:     gatewayapi.GrantType(externalIDP.GrantType),
	}

	if externalIDP.Basic != nil {
		idp.Basic = &gatewayapi.BasicAuthCredentials{
			Username: externalIDP.Basic.Username,
			Password: externalIDP.Basic.Password,
		}
	} else if externalIDP.Client != nil {
		idp.Client = &gatewayapi.OAuth2ClientCredentials{
			ClientId:     externalIDP.Client.ClientId,
			ClientSecret: externalIDP.Client.ClientSecret,
			ClientKey:    externalIDP.Client.ClientKey,
		}
	}

	return idp
}

func mapConsumerSecurity(apiSecurity *apiapi.SubscriberSecurity) *gatewayapi.ConsumeRouteSecurity {
	if apiSecurity == nil {
		return nil
	}

	security := &gatewayapi.ConsumeRouteSecurity{}

	if apiSecurity.M2M != nil {
		security.M2M = &gatewayapi.ConsumerMachine2MachineAuthentication{
			Scopes: apiSecurity.M2M.Scopes,
		}
		if apiSecurity.M2M.Client != nil {
			security.M2M.Client = &gatewayapi.OAuth2ClientCredentials{
				ClientId:     apiSecurity.M2M.Client.ClientId,
				ClientSecret: apiSecurity.M2M.Client.ClientSecret,
				ClientKey:    apiSecurity.M2M.Client.ClientKey,
			}
		} else if apiSecurity.M2M.Basic != nil {
			security.M2M.Basic = &gatewayapi.BasicAuthCredentials{
				Username: apiSecurity.M2M.Basic.Username,
				Password: apiSecurity.M2M.Basic.Password,
			}
		}
		security.M2M.Claims = mapClaims(apiSecurity.M2M.Claims)
	}

	return security
}

func mapTransformation(apiTransformation *apiapi.Transformation) *gatewayapi.Transformation {
	if apiTransformation == nil {
		return nil
	}

	transformation := &gatewayapi.Transformation{}

	if len(apiTransformation.Request.Headers.Remove) > 0 {
		transformation.Request.Headers.Remove = apiTransformation.Request.Headers.Remove
	}

	return transformation
}

func mapProviderRateLimitToGatewayRateLimit(apiRateLimitConfig *apiapi.RateLimitConfig) *gatewayapi.RateLimit {
	if apiRateLimitConfig == nil {
		return nil
	}
	return &gatewayapi.RateLimit{
		Limits:  mapLimitsToGatewayLimits(apiRateLimitConfig.Limits),
		Options: mapLimitOptionsToGatewayLimitOptions(apiRateLimitConfig.Options),
	}
}

func mapCircuitBreaker(cb *apiapi.CircuitBreaker) *gatewayapi.CircuitBreaker {
	circuitBreaker := &gatewayapi.CircuitBreaker{}
	if cb == nil {
		circuitBreaker.Enabled = false
	} else {
		circuitBreaker.Enabled = cb.Enabled
	}
	return circuitBreaker
}

func mapLimitsToGatewayLimits(apiLimits apiapi.Limits) gatewayapi.Limits {
	return gatewayapi.Limits{
		Second: apiLimits.Second,
		Minute: apiLimits.Minute,
		Hour:   apiLimits.Hour,
	}
}

func mapLimitOptionsToGatewayLimitOptions(apiRateLimitOptions apiapi.RateLimitOptions) gatewayapi.RateLimitOptions {
	return gatewayapi.RateLimitOptions{
		HideClientHeaders: apiRateLimitOptions.HideClientHeaders,
		FaultTolerant:     apiRateLimitOptions.FaultTolerant,
	}
}
