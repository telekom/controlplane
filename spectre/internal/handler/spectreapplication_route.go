// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

// reconcileSSERoutes manages SSE Route creation for cross-zone proxy routes and the primary route.
// When the SpectreApplication's zone is a proxy zone, it creates a proxy Route in the app's
// own zone and a primary Route in the backend zone. When the zone is local, only the primary
// Route is created. Once the Routes exist, the canonical SSE URL is published in status.
func (h *SpectreApplicationHandler) reconcileSSERoutes(
	ctx context.Context,
	obj *spectrev1.SpectreApplication,
	zone *adminv1.Zone,
	eventConfig *eventv1.EventConfig,
	subscriber *pubsubv1.Subscriber,
	appId string,
) error {
	logger := log.FromContext(ctx)

	obj.Status.ListenerRoute = nil
	obj.Status.ProxyRoute = nil
	obj.Status.SseUrl = ""

	backendZone, backendConfig, err := resolveSSEBackendZone(ctx, zone, eventConfig)
	if err != nil {
		return err
	}

	// When the app zone is a proxy zone, create a proxy SSE Route in the app's own
	// zone that forwards to the backend zone's gateway.
	if eventConfig.IsProxy() {
		proxyRoute, proxyErr := createSpectreSSEProxyRoute(ctx, obj, zone, backendZone, appId)
		if proxyErr != nil {
			return errors.Wrap(proxyErr, "failed to create proxy SSE Route")
		}
		obj.Status.ProxyRoute = ctypes.ObjectRefFromObject(proxyRoute)
		logger.V(1).Info("Created proxy SSE Route", "zone", zone.Name, "route", proxyRoute.Name)
	}

	// Primary SSE Route in the backend zone. When the app zone is a proxy zone, the
	// primary route needs trusted issuers from both zones so proxy-forwarded requests
	// are accepted.
	isProxyTarget := obj.Status.ProxyRoute != nil
	trustedIssuers := collectSpectreSSETrustedIssuers(backendZone, zone, isProxyTarget)

	primaryRoute, err := createSpectreSSEPrimaryRoute(ctx, obj, backendZone, backendConfig, appId, trustedIssuers, isProxyTarget)
	if err != nil {
		return errors.Wrap(err, "failed to create primary SSE Route")
	}
	obj.Status.ListenerRoute = ctypes.ObjectRefFromObject(primaryRoute)
	logger.V(1).Info("Created primary SSE Route", "zone", backendZone.Name, "route", primaryRoute.Name)

	sseUrl, err := makeSpectreSSEUrl(zone, appId, subscriber.Status.SubscriptionId)
	if err != nil {
		return err
	}
	obj.Status.SseUrl = sseUrl

	return nil
}

// makeSpectreSSEUrl builds the canonical SSE URL on the app's own zone gateway: the
// proxy-facing gateway for a proxy zone, the only gateway for a local zone. It returns
// an empty URL while the Subscriber has no SubscriptionId or the preset has no visible URL.
func makeSpectreSSEUrl(zone *adminv1.Zone, appId, subscriptionId string) (string, error) {
	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return "", ctrlerrors.BlockedErrorf("zone %q has no default gateway preset: %s", zone.Name, err)
	}
	baseUrl := preset.GetDefaultUrl()
	if subscriptionId == "" || baseUrl == "" {
		return "", nil
	}
	sseUrl, err := url.JoinPath(baseUrl, makeSpectreSSERoutePath(util.BuildListenerEventType(appId)), subscriptionId)
	if err != nil {
		return "", errors.Wrapf(err, "failed to build SSE URL for zone %q", zone.Name)
	}
	return sseUrl, nil
}

// resolveSSEBackendZone returns the zone (and its EventConfig) that runs the local
// Horizon SSE backend. For a local zone that is the zone itself. For a proxy zone
// it resolves the proxy's target zone, which must be a local (non-proxy) zone.
func resolveSSEBackendZone(ctx context.Context, zone *adminv1.Zone, eventConfig *eventv1.EventConfig) (*adminv1.Zone, *eventv1.EventConfig, error) {
	if !eventConfig.IsProxy() {
		return zone, eventConfig, nil
	}

	targetRef := eventConfig.Spec.Proxy.TargetZone
	c := cclient.ClientFromContextOrDie(ctx)

	targetZone := &adminv1.Zone{}
	if err := c.Get(ctx, targetRef.K8s(), targetZone); err != nil {
		return nil, nil, ctrlerrors.BlockedErrorf("target zone %q not found: %v", targetRef.String(), err)
	}

	if err := condition.EnsureReady(targetZone); err != nil {
		return nil, nil, ctrlerrors.BlockedErrorf("target zone %q is not ready", targetRef.String())
	}

	targetConfig, err := util.GetEventConfig(ctx, targetZone)
	if err != nil {
		return nil, nil, err
	}
	if !targetConfig.IsLocal() {
		return nil, nil, ctrlerrors.BlockedErrorf("target zone %q of proxy zone %q must be a local (non-proxy) zone", targetRef.Name, zone.Name)
	}

	return targetZone, targetConfig, nil
}

// createSpectreSSEPrimaryRoute creates the primary gateway Route for SSE in the backend zone.
func createSpectreSSEPrimaryRoute(
	ctx context.Context,
	obj *spectrev1.SpectreApplication,
	zone *adminv1.Zone,
	eventConfig *eventv1.EventConfig,
	appId string,
	trustedIssuers []string,
	isProxyTarget bool,
) (*gatewayv1.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	if zone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no gateway reference in status", zone.Name)
	}

	if !eventConfig.IsLocal() {
		return nil, ctrlerrors.BlockedErrorf("EventConfig %q for zone %q has no local backend", eventConfig.Name, zone.Name)
	}

	upstream, err := parseSSEUpstream(eventConfig.Spec.Local.ServerSendEventUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse ServerSendEventUrl %q", eventConfig.Spec.Local.ServerSendEventUrl)
	}

	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no default gateway preset: %s", zone.Name, err)
	}

	hostnames, paths := resolveSSEPaths(preset, appId)

	routeName := makeSpectreSSERouteName(appId)
	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.OwnerUidLabelKey:      string(obj.UID),
			config.DomainLabelKey:        "spectre",
			config.BuildLabelKey("zone"): zone.Name,
			config.BuildLabelKey("type"): "sse",
			config.BuildLabelKey("app"):  labelutil.NormalizeLabelValue(appId),
		}
		route.Spec = gatewayv1.RouteSpec{
			GatewayRef: *zone.Status.Gateway,
			Type:       gatewayv1.RouteTypePrimary,
			Backend:    gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayv1.Security{
				DisableAccessControl: true,
			},
			Buffering: gatewayv1.Buffering{
				DisableResponseBuffering: true,
			},
		}
		if len(trustedIssuers) > 0 {
			slices.Sort(trustedIssuers)
			route.Spec.Security.TrustedIssuers = slices.Compact(trustedIssuers)
		}
		if isProxyTarget {
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, gatewayv1.GatewayConsumerName)
		}
		if zone.Status.RealmName != "" {
			route.Spec.Security.RealmName = zone.Status.RealmName
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update SSE Route %q", routeName)
	}

	return route, nil
}

// createSpectreSSEProxyRoute creates a cross-zone proxy Route for SSE delivery in the
// app's own zone, forwarding to the backend zone's gateway.
func createSpectreSSEProxyRoute(
	ctx context.Context,
	obj *spectrev1.SpectreApplication,
	appZone *adminv1.Zone,
	backendZone *adminv1.Zone,
	appId string,
) (*gatewayv1.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	if appZone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no gateway reference in status", appZone.Name)
	}

	appPreset, err := appZone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no default gateway preset: %s", appZone.Name, err)
	}

	backendPreset, err := backendZone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("target zone %q has no default preset: %s", backendZone.Name, err)
	}

	eventType := util.BuildListenerEventType(appId)
	ssePath := makeSpectreSSERoutePath(eventType)

	upstream, err := gatewayUpstream(backendPreset, ssePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for SSE proxy route")
	}

	// Legacy alias requests on the proxy are forwarded to the backend's canonical path.
	hostnames, paths := resolveSSEPaths(appPreset, appId)

	routeName := makeSpectreSSEProxyRouteName(appId)
	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: appZone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.OwnerUidLabelKey:      string(obj.UID),
			config.DomainLabelKey:        "spectre",
			config.BuildLabelKey("zone"): appZone.Name,
			config.BuildLabelKey("type"): "sse-proxy",
			config.BuildLabelKey("app"):  labelutil.NormalizeLabelValue(appId),
		}
		route.Spec = gatewayv1.RouteSpec{
			GatewayRef: *appZone.Status.Gateway,
			Type:       gatewayv1.RouteTypeProxy,
			Backend:    gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayv1.Security{
				DisableAccessControl: true,
			},
			Buffering: gatewayv1.Buffering{
				DisableResponseBuffering: true,
			},
		}
		if appZone.Status.Links.Issuer != "" {
			route.Spec.Security.TrustedIssuers = []string{appZone.Status.Links.Issuer}
		}
		if appZone.Status.RealmName != "" {
			route.Spec.Security.RealmName = appZone.Status.RealmName
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update SSE proxy Route %q", routeName)
	}

	return route, nil
}

// collectSpectreSSETrustedIssuers builds the trusted issuer list for the primary SSE route.
// The backend zone's IDP issuer is always included. When the primary route is a proxy
// target, the app zone's LMS issuer is added so mesh-forwarded requests are accepted.
func collectSpectreSSETrustedIssuers(backendZone, appZone *adminv1.Zone, isProxyTarget bool) []string {
	var issuers []string
	if backendZone.Status.Links.Issuer != "" {
		issuers = append(issuers, backendZone.Status.Links.Issuer)
	}
	if isProxyTarget && appZone.Status.Links.LmsIssuer != "" {
		issuers = append(issuers, appZone.Status.Links.LmsIssuer)
	}
	return issuers
}

// gatewayUpstream builds a proxy Upstream pointing at a target preset's gateway URL
// joined with the given path.
func gatewayUpstream(preset *adminv1.GatewayConfigPreset, path string) (gatewayv1.Upstream, error) {
	full := preset.Urls[0].GetFullUrl()
	if path != "" {
		joined, err := url.JoinPath(full, path)
		if err != nil {
			return gatewayv1.Upstream{}, errors.Wrapf(err, "failed to build upstream URL for path %q", path)
		}
		full = joined
	}
	return parseSSEUpstream(full)
}

// makeSpectreSSERouteName returns a deterministic Route name for a Spectre listener's SSE endpoint.
func makeSpectreSSERouteName(appId string) string {
	return labelutil.NormalizeNameValue("spectre-sse--" + appId)
}

// makeSpectreSSEProxyRouteName returns a deterministic Route name for a Spectre SSE proxy route.
func makeSpectreSSEProxyRouteName(appId string) string {
	return labelutil.NormalizeNameValue("spectre-sse-proxy--" + appId)
}

// makeSpectreSSERoutePath builds the canonical SSE path for a Spectre listener event type.
func makeSpectreSSERoutePath(eventType string) string {
	return "/horizon/sse/v1/" + strings.ToLower(eventType)
}

// makeSpectreSSELegacyPath builds the legacy Spectre SSE path that existing
// listener URLs (/spectre-sse/<appId>/<subscriptionId>) still point at.
func makeSpectreSSELegacyPath(appId string) string {
	return "/spectre-sse/" + appId
}

// resolveSSEPaths returns the preset hostnames and the canonical plus legacy SSE
// paths for every preset URL. Canonical paths come first because Paths[0] drives
// the api_base_path header. Duplicates are dropped in order because RouteSpec
// Hostnames and Paths are sets.
func resolveSSEPaths(preset *adminv1.GatewayConfigPreset, appId string) (hostnames, paths []string) {
	hostnames, canonicalPaths := preset.ResolveHostnamesAndPaths(makeSpectreSSERoutePath(util.BuildListenerEventType(appId)))
	_, legacyPaths := preset.ResolveHostnamesAndPaths(makeSpectreSSELegacyPath(appId))
	return dedupeOrdered(hostnames), dedupeOrdered(slices.Concat(canonicalPaths, legacyPaths))
}

// dedupeOrdered returns values without duplicates, keeping first occurrences in order.
func dedupeOrdered(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// parseSSEUpstream parses a raw URL into a gateway Upstream.
func parseSSEUpstream(rawUrl string) (gatewayv1.Upstream, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return gatewayv1.Upstream{}, errors.Wrapf(err, "failed to parse URL %q", rawUrl)
	}
	return gatewayv1.Upstream{
		Scheme:   u.Scheme,
		Hostname: u.Hostname(),
		Port:     gatewayv1.GetPortOrDefaultFromScheme(u),
		Path:     u.Path,
	}, nil
}
