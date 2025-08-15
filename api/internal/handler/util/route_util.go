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
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	routeName := labelutil.NormalizeValue(apiBasePath)
	if realmName != "default" {
		routeName = realmName + "--" + routeName
	}
	return routeName
}

func CreateProxyRoute(ctx context.Context, downstreamZoneRef types.ObjectRef, upstreamZoneRef types.ObjectRef, apiBasePath, realmName string, opts ...CreateRouteOption) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	log := log.FromContext(ctx)

	options := &CreateRouteOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Downstream
	downstreamRealm, downstreamZone, err := GetRealmForZone(ctx, downstreamZoneRef, realmName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get downstream-realm %s", downstreamZoneRef.String())
	}

	// Upstream
	upstreamRealm, upstreamZone, err := GetRealmForZone(ctx, upstreamZoneRef, realmName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get upstream-realm %s", upstreamZoneRef.Name)
	}

	// Creating the Route
	proxyRoute := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeRouteName(apiBasePath, realmName),
			Namespace: downstreamRealm.Namespace,
		},
	}

	if options.ReturnReferenceOnly {
		// Return early with just the reference (name + namespace)
		return proxyRoute, nil
	}

	mutate := func() error {
		proxyRoute.Labels = map[string]string{
			apiapi.BasePathLabelKey:       labelutil.NormalizeValue(apiBasePath),
			config.BuildLabelKey("zone"):  labelutil.NormalizeValue(downstreamZone.GetName()),
			config.BuildLabelKey("realm"): labelutil.NormalizeValue(realmName),
			config.BuildLabelKey("type"):  "proxy",
		}

		downstream, err := downstreamRealm.AsDownstream(apiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create downstream")
		}

		upstream, err := AsUpstreamForProxyRoute(ctx, upstreamRealm, apiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create upstream")
		}

		proxyRoute.Spec = gatewayapi.RouteSpec{
			Realm: *types.ObjectRefFromObject(downstreamRealm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
		}

		log.Info("Creating proxy route", "route", proxyRoute.Name, "namespace", proxyRoute.Namespace, "failover", options.HasFailover())

		if options.HasServiceRateLimit() {
			proxyRoute.Spec.Traffic = gatewayapi.Traffic{
				RateLimit: mapProviderRateLimitToGatewayRateLimit(options.ServiceRateLimit),
			}
		}

		if options.IsFailoverSecondary() {
			proxyRoute.Labels[LabelFailoverSecondary] = "true"

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

func CreateRealRoute(ctx context.Context, downstreamZoneRef types.ObjectRef, apiExposure *apiapi.ApiExposure, realmName string) (*gatewayapi.Route, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

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
			apiapi.BasePathLabelKey:       labelutil.NormalizeValue(apiExposure.Spec.ApiBasePath),
			config.BuildLabelKey("zone"):  labelutil.NormalizeValue(zone.Name),
			config.BuildLabelKey("realm"): labelutil.NormalizeValue(downstreamRealm.Name),
			config.BuildLabelKey("type"):  "real",
		}

		downstream, err := downstreamRealm.AsDownstream(apiExposure.Spec.ApiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create downstream")
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
			Realm:     *types.ObjectRefFromObject(downstreamRealm),
			Upstreams: gatewayUpstreams,
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
			Traffic: gatewayapi.Traffic{},
		}
		route.Spec.Transformation = mapTransformation(apiExposure.Spec.Transformation)
		route.Spec.Security = mapSecurity(apiExposure.Spec.Security)

		if apiExposure.HasProviderRateLimit() {
			route.Spec.Traffic.RateLimit = mapProviderRateLimitToGatewayRateLimit(apiExposure.Spec.Traffic.RateLimit.Provider)
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
