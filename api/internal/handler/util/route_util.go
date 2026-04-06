// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// GatewayConsumerName is the name of the gateway mesh-client that is used to proxy requests across zones.
	// It must be added to DefaultConsumers on routes that are the target of a cross-zone proxy.
	GatewayConsumerName = "gateway"
)

var (
	LabelFailoverSecondary = config.BuildLabelKey("failover.secondary")
)

type CreateRouteOptions struct {
	FailoverUpstreams   []apiapi.Upstream
	FailoverZone        types.ObjectRef
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
}

type CreateRouteOption func(*CreateRouteOptions)

type CreateConsumeRouteOptions struct {
	ConsumerRateLimit *apiapi.Limits // Rate limit configuration for the consumer
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

// WithFailoverZone sets the failover zone for the route.
// A Proxy-Route created using CreateProxyRoute with this option will have the failover zone set.
// This will result in a Proxy-Route that will proxy requests to the failover zone (secondary route).
func WithFailoverZone(failoverZone types.ObjectRef) CreateRouteOption {
	return func(opts *CreateRouteOptions) {
		opts.FailoverZone = failoverZone
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

// IsFailoverSecondary checks if the route is a failover secondary route.
// This means that this route has the real upstream as a failover upstream.
func (o *CreateRouteOptions) IsFailoverSecondary() bool {
	return len(o.FailoverUpstreams) > 0
}

// HasFailover checks if the route has a failover zone configured.
// This means that this route is used as a proxy to the failover zone.
func (o *CreateRouteOptions) HasFailover() bool {
	return o.FailoverZone.Name != "" && o.FailoverZone.Namespace != ""
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

func MakeRouteName(apiBasePath, realmName string) string {
	return labelutil.NormalizeValue(apiBasePath)
}

func CreateProxyRoute(ctx context.Context, downstreamZoneRef types.ObjectRef, upstreamZoneRef types.ObjectRef, apiBasePath, realmName string, opts ...CreateRouteOption) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	log := log.FromContext(ctx)

	options := &CreateRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Downstream: Use the provided realm (could be DTC if failover is enabled)
	// This allows subscribers to access the API via DTC gateway hosts
	downstreamRealm, downstreamZone, err := GetRealmForZone(ctx, downstreamZoneRef, realmName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get downstream-realm %s", downstreamZoneRef.String())
	}

	// Upstream: ALWAYS use the default realm of the provider zone
	// DTC realm is NOT guaranteed to exist in the upstream zone, and proxy routes
	// need to forward to the specific provider zone's default gateway
	upstreamRealmName := contextutil.EnvFromContextOrDie(ctx) // default realm name
	upstreamRealm, upstreamZone, err := GetRealmForZone(ctx, upstreamZoneRef, upstreamRealmName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get upstream-realm %s", upstreamZoneRef.Name)
	}

	// Creating the Route
	routeName := MakeRouteName(apiBasePath, realmName)

	proxyRoute := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeNameValue(routeName),
			Namespace: downstreamRealm.Namespace,
		},
	}

	if options.ReturnReferenceOnly {
		// Return early with just the reference (name + namespace)
		return proxyRoute, nil
	}

	mutate := func() error {
		proxyRoute.Labels = map[string]string{
			apiapi.BasePathLabelKey:       labelutil.NormalizeLabelValue(apiBasePath),
			config.BuildLabelKey("zone"):  labelutil.NormalizeValue(downstreamZone.GetName()),
			config.BuildLabelKey("realm"): labelutil.NormalizeValue(realmName),
			config.BuildLabelKey("type"):  "proxy",
		}

		// Use AsDownstreams to get all downstreams from the realm
		// This is critical for DTC realms which have multiple URLs and issuers
		downstreams, err := downstreamRealm.AsDownstreams(apiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create downstreams")
		}

		upstream, err := AsUpstreamForProxyRoute(ctx, upstreamRealm, apiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create upstream")
		}

		proxyRoute.Spec = gatewayapi.RouteSpec{
			Realm:       *types.ObjectRefFromObject(downstreamRealm),
			Upstreams:   []gatewayapi.Upstream{upstream},
			Downstreams: downstreams,
		}

		log.Info("Creating proxy route", "route", proxyRoute.Name, "namespace", proxyRoute.Namespace, "failover", options.HasFailover())

		if options.HasServiceRateLimit() {
			proxyRoute.Spec.Traffic = gatewayapi.Traffic{
				RateLimit: mapProviderRateLimitToGatewayRateLimit(options.ServiceRateLimit),
			}
		}

		if options.IsFailoverSecondary() {
			proxyRoute.Labels[LabelFailoverSecondary] = "true"

			// A failover secondary route is the target of cross-zone proxy requests,
			// so the gateway mesh-client must be allowed to access it.
			if proxyRoute.Spec.Security == nil {
				proxyRoute.Spec.Security = &gatewayapi.Security{}
			}
			proxyRoute.Spec.Security.DefaultConsumers = append(proxyRoute.Spec.Security.DefaultConsumers, GatewayConsumerName)

			failoverUpstreams := make([]gatewayapi.Upstream, 0, len(options.FailoverUpstreams))
			for _, rawUpstream := range options.FailoverUpstreams {
				failoverUpstream, err := AsUpstreamForRealRoute(ctx, rawUpstream.Url, rawUpstream.Weight)
				if err != nil {
					return errors.Wrapf(err, "failed to create failover upstream %s", rawUpstream.Url)
				}
				failoverUpstreams = append(failoverUpstreams, failoverUpstream)
			}

			proxyRoute.Spec.Traffic = gatewayapi.Traffic{
				Failover: &gatewayapi.Failover{
					TargetZoneName: upstreamZone.Name,
					Upstreams:      failoverUpstreams,
				},
			}

			// Add the provided security config (mostly copied from primary-route)
			// to the failover config of the secondary route
			if options.FailoverSecurity != nil {
				proxyRoute.Spec.Traffic.Failover.Security = mapSecurity(options.FailoverSecurity)
			}
		}

		if options.HasFailover() {
			proxyRoute.Labels[config.BuildLabelKey("failover.zone")] = labelutil.NormalizeValue(options.FailoverZone.Name)
			failoverUpstreamRealm, _, err := GetRealmForZone(ctx, options.FailoverZone, realmName)
			if err != nil {
				return errors.Wrapf(err, "failed to get failover zone %s", options.FailoverZone.String())
			}
			failoverUpstream, err := AsUpstreamForProxyRoute(ctx, failoverUpstreamRealm, apiBasePath)

			proxyRoute.Spec.Traffic = gatewayapi.Traffic{
				Failover: &gatewayapi.Failover{
					TargetZoneName: upstreamZone.Name,
					Upstreams: []gatewayapi.Upstream{
						failoverUpstream,
					},
				},
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

// CleanupProxyRoute deletes the route only if no other subscriptions (size > 1) for this route exist
func CleanupProxyRoute(ctx context.Context, routeRef *types.ObjectRef, opts ...CreateRouteOption) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	if routeRef == nil {
		return nil
	}
	log := log.FromContext(ctx).WithValues("route.name", routeRef.Name, "route.namespace", routeRef.Namespace)

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
		log.V(1).Info("🫷 Not deleting route as it is a real route")
		return nil
	}

	if route.GetLabels()[LabelFailoverSecondary] == "true" { // DO NOT DELETE FAILOVER ROUTES
		log.V(1).Info("🫷 Not deleting route as it is a failover secondary")
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
		log.Info("🫷 Not deleting route as more than 1 subscriptions exists")
		return nil
	}

	log.Info("🧹 Deleting route as no more subscriptions exist")

	err = scopedClient.Delete(ctx, route)
	if err != nil {
		return errors.Wrapf(err, "failed to delete route")
	}
	log.Info("✅ Successfully deleted obsolete route")

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

func CreateRealRoute(ctx context.Context, downstreamZoneRef types.ObjectRef, apiExposure *apiapi.ApiExposure, realmName string, opts ...CreateRouteOption) (*gatewayapi.Route, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	options := &CreateRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// get referenced Zone from exposure
	zone, err := GetZone(ctx, scopedClient, downstreamZoneRef.K8s())
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Unable to get zone %s", downstreamZoneRef.String()))
	}
	downstreamRealmRef := client.ObjectKey{
		Name:      realmName,
		Namespace: zone.Status.Namespace,
	}

	downstreamRealm, err := GetRealm(ctx, downstreamRealmRef)
	if err != nil {
		return nil, err
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeRouteName(apiExposure.Spec.ApiBasePath, realmName),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			apiapi.BasePathLabelKey:       labelutil.NormalizeLabelValue(apiExposure.Spec.ApiBasePath),
			config.BuildLabelKey("zone"):  labelutil.NormalizeValue(zone.Name),
			config.BuildLabelKey("realm"): labelutil.NormalizeValue(downstreamRealm.Name),
			config.BuildLabelKey("type"):  "real",
		}

		// Use AsDownstreams to get all downstreams from the realm
		// This is critical for DTC realms which have multiple URLs and issuers
		downstreams, err := downstreamRealm.AsDownstreams(apiExposure.Spec.ApiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create downstreams")
		}

		gatewayUpstreams := make([]gatewayapi.Upstream, 0, len(apiExposure.Spec.Upstreams))
		for _, upstream := range apiExposure.Spec.Upstreams {
			gatewayUpstream, err := AsUpstreamForRealRoute(ctx, upstream.Url, upstream.Weight)
			if err != nil {
				return errors.Wrapf(err, "failed to create upstream for URL %s", upstream.Url)
			}
			gatewayUpstreams = append(gatewayUpstreams, gatewayUpstream)
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm:       *types.ObjectRefFromObject(downstreamRealm),
			Upstreams:   gatewayUpstreams,
			Downstreams: downstreams,
			Traffic:     gatewayapi.Traffic{},
		}
		route.Spec.Transformation = mapTransformation(apiExposure.Spec.Transformation)
		route.Spec.Security = mapSecurity(apiExposure.Spec.Security)

		if options.IsProxyTarget {
			// If this Route is the target of a cross-zone proxy Route,
			// the gateway mesh-client must be allowed to access it.
			if route.Spec.Security == nil {
				route.Spec.Security = &gatewayapi.Security{}
			}
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, GatewayConsumerName)
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

func CreateConsumeRoute(ctx context.Context, apiSub *apiapi.ApiSubscription, downstreamZoneRef types.ObjectRef, routeRef types.ObjectRef, clientId string, opts ...CreateConsumeRouteOption) (*gatewayapi.ConsumeRoute, error) {
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

func mapSecurity(apiSecurity *apiapi.Security) *gatewayapi.Security {
	if apiSecurity == nil {
		return nil
	}

	security := &gatewayapi.Security{}

	if apiSecurity.M2M != nil {
		security.M2M = &gatewayapi.Machine2MachineAuthentication{
			Scopes: apiSecurity.M2M.Scopes,
		}
		if apiSecurity.M2M.ExternalIDP != nil {
			security.M2M.ExternalIDP = &gatewayapi.ExternalIdentityProvider{
				TokenEndpoint: apiSecurity.M2M.ExternalIDP.TokenEndpoint,
				TokenRequest:  apiSecurity.M2M.ExternalIDP.TokenRequest,
				GrantType:     apiSecurity.M2M.ExternalIDP.GrantType,
			}
			if apiSecurity.M2M.ExternalIDP.Basic != nil {
				security.M2M.ExternalIDP.Basic = &gatewayapi.BasicAuthCredentials{
					Username: apiSecurity.M2M.ExternalIDP.Basic.Username,
					Password: apiSecurity.M2M.ExternalIDP.Basic.Password,
				}
			} else if apiSecurity.M2M.ExternalIDP.Client != nil {
				security.M2M.ExternalIDP.Client = &gatewayapi.OAuth2ClientCredentials{
					ClientId:     apiSecurity.M2M.ExternalIDP.Client.ClientId,
					ClientSecret: apiSecurity.M2M.ExternalIDP.Client.ClientSecret,
					ClientKey:    apiSecurity.M2M.ExternalIDP.Client.ClientKey,
				}
			}
		}
		if apiSecurity.M2M.Basic != nil {
			security.M2M.Basic = &gatewayapi.BasicAuthCredentials{
				Username: apiSecurity.M2M.Basic.Username,
				Password: apiSecurity.M2M.Basic.Password,
			}
		}

	}

	return security
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
