// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

const exposureVariantTagPrefix = "variant--"

// CreateAgenticRoute creates the primary gateway Route for an MCP exposure.
// The Route is created in the zone's namespace with buffering disabled for streaming.
// Follows the same preset-based routing pattern as the API domain's CreateRealRoute.
// hasLocalCallers is true if consumers in the exposure zone call the Route directly.
// telecontextConsumer is set only if Telecontext is in the exposure zone.
func CreateAgenticRoute(
	ctx context.Context,
	exposure *agenticv1.AgenticExposure,
	zone *adminv1.Zone,
	hasLocalCallers bool,
	isTargetOfProxy bool,
	telecontextConsumer string,
	crossZoneLmsIssuers []string,
) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// 1. Get AI Gateway preset and gateway ref
	preset, err := zone.Spec.SelectPreset(adminv1.GatewayTypeAI)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no AI Gateway preset: %v", zone.Name, err)
	}
	presetStatus, err := zone.Status.GetPreset(preset.Name)
	if err != nil || presetStatus.GatewayRef == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no AI Gateway configured", zone.Name)
	}

	// 2. Map upstreams
	gatewayUpstreams, err := MapUpstreamsToGateway(exposure.Spec.Upstreams)
	if err != nil {
		return nil, errors.Wrap(err, "failed to map upstreams")
	}

	// 3. Resolve hostnames and paths from preset
	hostnames, paths := preset.ResolveHostnamesAndPaths(exposure.Spec.BasePath)

	// 4. Create or update the Route
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeAgenticRouteName(exposure.Spec.BasePath),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:             LabelValueDomain,
			agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(exposure.Spec.BasePath),
			config.BuildLabelKey("zone"):      zone.Name,
			config.BuildLabelKey("type"):      "mcp",
		}

		route.Spec = gatewayapi.RouteSpec{
			AdditionalTags: []string{exposureVariantTagPrefix + labelutil.NormalizeNameValue(string(exposure.Spec.Variant))},
			GatewayRef:     *presetStatus.GatewayRef,
			Type:           gatewayapi.RouteTypePrimary,
			Backend:        gatewayapi.Backend{Upstreams: gatewayUpstreams},
			Hostnames:      hostnames,
			Paths:          paths,
			Traffic:        MapTrafficToGateway(&exposure.Spec.Traffic),
			// Critical: disable buffering for MCP streaming (SSE)
			Buffering: gatewayapi.Buffering{
				DisableRequestBuffering:  true,
				DisableResponseBuffering: true,
			},
		}

		// Apply security settings
		if exposure.Spec.Security != nil {
			route.Spec.Security = MapSecurityToGateway(exposure.Spec.Security)
		} else {
			route.Spec.Security = gatewayapi.Security{}
		}
		route.Spec.Security.RealmName = zone.Status.RealmName

		// Set trusted issuers: only add the exposure zone's own IDP issuer when
		// there are local callers. Cross-zone proxy gateways
		// are trusted via their LMS issuers instead.
		var trustedIssuers []string
		if hasLocalCallers && presetStatus.Links.Issuer != "" {
			trustedIssuers = append(trustedIssuers, presetStatus.Links.Issuer)
		}
		trustedIssuers = append(trustedIssuers, crossZoneLmsIssuers...)
		if len(trustedIssuers) > 0 {
			route.Spec.Security.TrustedIssuers = trustedIssuers
		}

		// Apply transformation settings if provided
		if exposure.Spec.Transformation != nil {
			route.Spec.Transformation = MapTransformationToGateway(exposure.Spec.Transformation)
		}

		// If this Route is a target of proxy Routes, allow gateway mesh-client access
		if isTargetOfProxy {
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, GatewayConsumerName)
		}

		// If a platform consumer (e.g. Telecontext) should get automatic access
		if telecontextConsumer != "" {
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, telecontextConsumer)
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update MCP Route %s/%s", route.Namespace, route.Name)
	}

	return route, nil
}

// CreateAgenticProxyRoute creates a cross-zone proxy Route for MCP delivery.
// The Route is created in the namespace of the proxy zone (a subscriber zone or the
// Telecontext zone) and points upstream to the provider zone's AI Gateway.
// Follows the same preset-based routing pattern as the API domain's CreateProxyRoute.
// extraConsumers get access to the Route in addition to the gateway mesh client.
func CreateAgenticProxyRoute(
	ctx context.Context,
	basePath string,
	variant agenticv1.AgenticVariant,
	proxyZone *adminv1.Zone,
	providerZone *adminv1.Zone,
	extraConsumers ...string,
) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// 1. Resolve proxy zone's AI Gateway preset (for downstream hostnames/paths)
	proxyPreset, err := proxyZone.Spec.SelectPreset(adminv1.GatewayTypeAI)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("proxy zone %q has no AI Gateway preset: %v", proxyZone.Name, err)
	}
	proxyPresetStatus, err := proxyZone.Status.GetPreset(proxyPreset.Name)
	if err != nil || proxyPresetStatus.GatewayRef == nil {
		return nil, ctrlerrors.BlockedErrorf("proxy zone %q has no AI Gateway configured", proxyZone.Name)
	}

	// 2. Resolve provider zone's AI Gateway preset (for upstream URL)
	providerPreset, err := providerZone.Spec.SelectPreset(adminv1.GatewayTypeAI)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("provider zone %q has no AI Gateway preset: %v", providerZone.Name, err)
	}

	// 3. Build upstream from provider zone's preset URL
	upstreamUrl, err := url.JoinPath(providerPreset.GetDefaultURL(), basePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build upstream URL for proxy route")
	}
	upstream, err := AsUpstream(upstreamUrl, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy route")
	}

	// 4. Build downstream hostnames/paths from proxy preset
	hostnames, paths := proxyPreset.ResolveHostnamesAndPaths(basePath)

	// 5. Create or update the proxy Route in the proxy zone's namespace
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeAgenticRouteName(basePath),
			Namespace: proxyZone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:             LabelValueDomain,
			agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
			config.BuildLabelKey("zone"):      proxyZone.Name,
			config.BuildLabelKey("type"):      "mcp-proxy",
		}

		route.Spec = gatewayapi.RouteSpec{
			AdditionalTags: []string{exposureVariantTagPrefix + labelutil.NormalizeNameValue(string(variant))},
			GatewayRef:     *proxyPresetStatus.GatewayRef,
			Type:           gatewayapi.RouteTypeProxy,
			Backend:        gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:      hostnames,
			Paths:          paths,
			Security: gatewayapi.Security{
				DefaultConsumers: append([]string{GatewayConsumerName}, extraConsumers...),
				RealmName:        proxyZone.Status.RealmName,
			},
			Traffic: gatewayapi.Traffic{},
			// Critical: disable buffering for MCP streaming
			Buffering: gatewayapi.Buffering{
				DisableRequestBuffering:  true,
				DisableResponseBuffering: true,
			},
		}

		// Set trusted issuers from proxy zone's IDP for consumer token validation
		if proxyPresetStatus.Links.Issuer != "" {
			route.Spec.Security.TrustedIssuers = []string{proxyPresetStatus.Links.Issuer}
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update MCP proxy Route %s/%s", route.Namespace, route.Name)
	}

	return route, nil
}

// CleanupOldAgenticRoutes uses the JanitorClient's Cleanup() to delete stale MCP Routes
// for the given basePath that were NOT created/updated in this reconciliation cycle.
func CleanupOldAgenticRoutes(ctx context.Context, basePath string) (int, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	deleted, err := c.Cleanup(ctx, &gatewayapi.RouteList{}, []client.ListOption{
		client.MatchingLabels{
			agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
		},
	})
	if err != nil {
		return deleted, errors.Wrapf(err, "failed to cleanup old MCP Routes for basePath %q", basePath)
	}

	return deleted, nil
}

// DeleteRouteIfExists deletes a Route by ObjectRef, ignoring NotFound errors.
func DeleteRouteIfExists(ctx context.Context, ref *ctypes.ObjectRef) error {
	c := cclient.ClientFromContextOrDie(ctx)
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		},
	}
	err := c.Delete(ctx, route)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete Route %q", ref.String())
	}
	return nil
}

// AsUpstream converts a raw URL to a gateway Upstream struct.
func AsUpstream(rawUrl string, weight int32) (gatewayapi.Upstream, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return gatewayapi.Upstream{}, errors.Wrapf(err, "failed to parse URL %s", rawUrl)
	}

	return gatewayapi.Upstream{
		Scheme:   u.Scheme,
		Hostname: u.Hostname(),
		Port:     gatewayapi.GetPortOrDefaultFromScheme(u),
		Path:     u.Path,
		Weight:   weight,
	}, nil
}

func MapUpstreamsToGateway(upstreams []agenticv1.Upstream) ([]gatewayapi.Upstream, error) {
	mapped := make([]gatewayapi.Upstream, 0, len(upstreams))
	for _, upstream := range upstreams {
		gatewayUpstream, err := AsUpstream(upstream.Url, int32(upstream.Weight)) //nolint:gosec // weight is a small positive integer
		if err != nil {
			return nil, errors.Wrapf(err, "failed to map upstream %s", upstream.Url)
		}
		mapped = append(mapped, gatewayUpstream)
	}
	return mapped, nil
}

func MapSecurityToGateway(security *agenticv1.Security) gatewayapi.Security {
	if security == nil {
		return gatewayapi.Security{}
	}

	gatewaySecurity := gatewayapi.Security{}
	if security.M2M != nil {
		gatewaySecurity.M2M = &gatewayapi.Machine2MachineAuthentication{
			Scopes: append([]string(nil), security.M2M.Scopes...),
		}
		if security.M2M.ExternalIDP != nil {
			gatewaySecurity.M2M.ExternalIDP = &gatewayapi.ExternalIdentityProvider{
				TokenEndpoint: security.M2M.ExternalIDP.TokenEndpoint,
				TokenRequest:  gatewayapi.TokenRequestMethod(security.M2M.ExternalIDP.TokenRequest),
				GrantType:     gatewayapi.GrantType(security.M2M.ExternalIDP.GrantType),
				Basic:         mapBasicAuthToGateway(security.M2M.ExternalIDP.Basic),
				Client:        mapOAuth2ClientToGateway(security.M2M.ExternalIDP.Client),
			}
		}
		gatewaySecurity.M2M.Basic = mapBasicAuthToGateway(security.M2M.Basic)
	}

	return gatewaySecurity
}

func MapTrafficToGateway(traffic *agenticv1.Traffic) gatewayapi.Traffic {
	if traffic == nil {
		return gatewayapi.Traffic{}
	}

	gatewayTraffic := gatewayapi.Traffic{}
	if traffic.RateLimit != nil && traffic.RateLimit.Provider != nil {
		gatewayTraffic.RateLimit = &gatewayapi.RateLimit{
			Limits:  mapLimitsToGateway(traffic.RateLimit.Provider.Limits),
			Options: mapRateLimitOptionsToGateway(traffic.RateLimit.Provider.Options),
		}
	}
	if traffic.CircuitBreaker != nil {
		gatewayTraffic.CircuitBreaker = &gatewayapi.CircuitBreaker{
			Enabled: traffic.CircuitBreaker.Enabled,
		}
	}

	return gatewayTraffic
}

func MapSubscriberSecurityToGateway(security *agenticv1.SubscriberSecurity) *gatewayapi.ConsumeRouteSecurity {
	if security == nil {
		return nil
	}

	gatewaySecurity := &gatewayapi.ConsumeRouteSecurity{}
	if security.M2M != nil {
		gatewaySecurity.M2M = &gatewayapi.ConsumerMachine2MachineAuthentication{
			Scopes: append([]string(nil), security.M2M.Scopes...),
			Client: mapOAuth2ClientToGateway(security.M2M.Client),
			Basic:  mapBasicAuthToGateway(security.M2M.Basic),
		}
	}

	return gatewaySecurity
}

func MapSubscriberTrafficToGateway(traffic *agenticv1.SubscriberTraffic) *gatewayapi.ConsumeRouteTraffic {
	if traffic == nil {
		return nil
	}

	return nil
}

func mapBasicAuthToGateway(credentials *agenticv1.BasicAuthCredentials) *gatewayapi.BasicAuthCredentials {
	if credentials == nil {
		return nil
	}

	return &gatewayapi.BasicAuthCredentials{
		Username: credentials.Username,
		Password: credentials.Password,
	}
}

func mapOAuth2ClientToGateway(credentials *agenticv1.OAuth2ClientCredentials) *gatewayapi.OAuth2ClientCredentials {
	if credentials == nil {
		return nil
	}

	return &gatewayapi.OAuth2ClientCredentials{
		ClientId:     credentials.ClientId,
		ClientSecret: credentials.ClientSecret,
		ClientKey:    credentials.ClientKey,
	}
}

func mapLimitsToGateway(limits agenticv1.Limits) gatewayapi.Limits {
	return gatewayapi.Limits{
		Second: limits.Second,
		Minute: limits.Minute,
		Hour:   limits.Hour,
	}
}

func mapRateLimitOptionsToGateway(options agenticv1.RateLimitOptions) gatewayapi.RateLimitOptions {
	return gatewayapi.RateLimitOptions{
		HideClientHeaders: options.HideClientHeaders,
		FaultTolerant:     options.FaultTolerant,
	}
}

func MapTransformationToGateway(transformation *agenticv1.Transformation) *gatewayapi.Transformation {
	if transformation == nil {
		return nil
	}

	return &gatewayapi.Transformation{
		Request: gatewayapi.RequestResponseTransformation{
			Headers: gatewayapi.HeaderTransformation{
				Remove: transformation.Request.Headers.Remove,
				Add:    transformation.Request.Headers.Add,
			},
		},
	}
}
