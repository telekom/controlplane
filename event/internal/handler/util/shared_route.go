// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"
	"slices"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// DeleteRouteIfExists fetches a Route by ObjectRef and deletes it if found.
// Returns nil if the Route is already gone (NotFound).
func DeleteRouteIfExists(ctx context.Context, ref *ctypes.ObjectRef) error {
	if ref == nil {
		return nil
	}

	c := cclient.ClientFromContextOrDie(ctx)

	route := &gatewayapi.Route{}
	err := c.Get(ctx, ref.K8s(), route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get Route %q", ref.String())
	}

	if err := c.Delete(ctx, route); err != nil {
		return errors.Wrapf(err, "failed to delete Route %q", ref.String())
	}

	return nil
}

// parseUpstream parses a raw URL string into a gateway Upstream.
func parseUpstream(rawUrl string) (gatewayapi.Upstream, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return gatewayapi.Upstream{}, errors.Wrapf(err, "failed to parse URL %s", rawUrl)
	}
	return gatewayapi.Upstream{
		Scheme:   u.Scheme,
		Hostname: u.Hostname(),
		Port:     gatewayapi.GetPortOrDefaultFromScheme(u),
		Path:     u.Path,
	}, nil
}

type Options struct {
	Owner metav1.Object

	IsProxyTarget bool

	// ExtraConsumers are additional client names appended to the route's
	// DefaultConsumers regardless of IsProxyTarget. Callback routes use this to
	// always trust the Horizon callback client (CallbackClientName).
	ExtraConsumers []string

	// TrustedIssuers is the list of trusted token issuers for this route.
	// For primary routes: includes the zone's IDP issuer + LMS issuers from proxy zones.
	// For proxy routes: includes the source zone's LMS issuer (mesh-client authentication).
	TrustedIssuers []string

	// RealmName is the identity realm name used by the Jumper sidecar for
	// Last-Mile-Security token issuance. Typically equals the environment name.
	RealmName string
}

type Option func(*Options)

func WithOwner(owner metav1.Object) Option {
	return func(o *Options) {
		o.Owner = owner
	}
}

func WithProxyTarget(isProxyTarget bool) Option {
	return func(o *Options) {
		o.IsProxyTarget = isProxyTarget
	}
}

// WithCallbackConsumer marks the route as a callback route, so the Horizon
// callback client (CallbackClientName) is added to its DefaultConsumers.
func WithCallbackConsumer() Option {
	return func(o *Options) {
		o.ExtraConsumers = append(o.ExtraConsumers, CallbackClientName)
	}
}

// WithTrustedIssuers sets the trusted token issuers for the route.
// These issuers are used by the gateway's JWT plugin to validate incoming tokens.
func WithTrustedIssuers(issuers []string) Option {
	return func(o *Options) {
		o.TrustedIssuers = issuers
	}
}

// WithRealmName sets the identity realm name on the route's Security.
// The Jumper sidecar uses this to determine which realm to use for LMS token issuance.
func WithRealmName(realmName string) Option {
	return func(o *Options) {
		o.RealmName = realmName
	}
}

func (o *Options) apply(ctx context.Context, route *gatewayapi.Route) error {
	c := cclient.ClientFromContextOrDie(ctx)
	if o.Owner != nil {
		return controllerutil.SetControllerReference(o.Owner, route, c.Scheme())
	}
	return nil
}

// applySecurity sets TrustedIssuers and RealmName on the route's Security from options.
func (o *Options) applySecurity(route *gatewayapi.Route) {
	if len(o.TrustedIssuers) > 0 {
		slices.Sort(o.TrustedIssuers)
		route.Spec.Security.TrustedIssuers = slices.Compact(o.TrustedIssuers)
	}
	if o.RealmName != "" {
		route.Spec.Security.RealmName = o.RealmName
	}
}

// RouteDownstreamURL constructs the external-facing URL from a Route's
// Hostnames[0] and Paths[0]. Returns empty string if either slice is empty.
func RouteDownstreamURL(route *gatewayapi.Route) string {
	if len(route.Spec.Hostnames) == 0 || len(route.Spec.Paths) == 0 {
		return ""
	}
	return "https://" + route.Spec.Hostnames[0] + route.Spec.Paths[0]
}

// resolvePreset returns the zone's default gateway preset and verifies the zone
// has a gateway reference in its status. Used for the downstream (own) zone of a
// Route, where both the preset (hostnames/paths) and the gateway ref are needed.
func resolvePreset(zone *adminv1.Zone) (*adminv1.GatewayConfigPreset, error) {
	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no default preset: %s", zone.Name, err)
	}
	if zone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no gateway reference in status", zone.Name)
	}
	return preset, nil
}

// targetPreset returns a zone's default preset for use as a proxy Route's
// upstream. Unlike resolvePreset it does NOT require Status.Gateway: the target
// zone of a cross-zone proxy only contributes its public URL, not a GatewayRef,
// so its gateway status being unpopulated must not block the proxy Route.
func targetPreset(zone *adminv1.Zone) (*adminv1.GatewayConfigPreset, error) {
	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("target zone %q has no default preset: %s", zone.Name, err)
	}
	return preset, nil
}

// gatewayUpstream builds a proxy Upstream pointing at a target preset's gateway URL,
// optionally joined with path. path=="" yields the base URL only (publish-proxy relies
// on the gateway preserving the request path when the upstream carries none).
func gatewayUpstream(preset *adminv1.GatewayConfigPreset, path string) (gatewayapi.Upstream, error) {
	full := preset.Urls[0].GetFullUrl()
	if path != "" {
		joined, err := url.JoinPath(full, path)
		if err != nil {
			return gatewayapi.Upstream{}, errors.Wrapf(err, "failed to build upstream URL for path %q", path)
		}
		full = joined
	}
	return parseUpstream(full)
}

// finalizeRoute runs the shared mutator tail every builder needs: apply owner,
// let build set labels+spec, apply security options, add the gateway mesh-client
// consumer when the route is a proxy target (o.IsProxyTarget) and any callback
// consumers (o.ExtraConsumers), then CreateOrUpdate. build fills route.Spec
// inline; nothing here re-models RouteSpec.
func finalizeRoute(ctx context.Context, route *gatewayapi.Route, o *Options, build func() error) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	wrapped := func() error {
		if err := o.apply(ctx, route); err != nil {
			return errors.Wrap(err, "failed to apply options to Route")
		}
		if err := build(); err != nil {
			return err
		}
		o.applySecurity(route)
		if o.IsProxyTarget {
			// Cross-zone meshing: the target of a proxy route is accessed by the
			// gateway mesh-client, so it must trust GatewayConsumerName.
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, gatewayapi.GatewayConsumerName)
		}
		if len(o.ExtraConsumers) > 0 {
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, o.ExtraConsumers...)
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, route, wrapped); err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Route %q", ctypes.ObjectRefFromObject(route).String())
	}
	return route, nil
}

// buildCrossZoneProxyRoute builds a single cross-zone proxy Route: created in
// sourceZone's namespace, upstream pointing at targetZone's gateway path. kind is
// the route family ("callback", "voyager") used for the route name/path (via the
// passed routeName/path) and the "<kind>-proxy" type label.
func buildCrossZoneProxyRoute(
	ctx context.Context,
	sourceZone, targetZone *adminv1.Zone,
	kind, routeName, path string,
	disableAccessControl bool,
	opts ...Option,
) (*gatewayapi.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	sourcePreset, err := resolvePreset(sourceZone)
	if err != nil {
		return nil, err
	}

	tgtPreset, err := targetPreset(targetZone)
	if err != nil {
		return nil, err
	}

	upstream, err := gatewayUpstream(tgtPreset, path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create upstream for proxy %s Route", kind)
	}

	hostnames, paths := sourcePreset.ResolveHostnamesAndPaths(path)

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: sourceZone.Status.Namespace,
		},
	}

	build := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): sourceZone.Name,
			config.BuildLabelKey("type"): kind + "-proxy",
		}
		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *sourceZone.Status.Gateway,
			Type:       gatewayapi.RouteTypeProxy,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayapi.Security{
				DisableAccessControl: disableAccessControl,
			},
		}
		return nil
	}
	return finalizeRoute(ctx, route, options, build)
}

// proxyRouteBuilder builds a single proxy Route from a source and target zone.
// The per-kind builders (CreateProxyCallbackRoute, CreateProxyVoyagerRoute, ...)
// satisfy it and are fanned out by createProxyRoutes.
type proxyRouteBuilder func(ctx context.Context, src, tgt *adminv1.Zone, opts ...Option) (*gatewayapi.Route, error)

// createProxyRoutes fans out a single-route proxy builder over the mesh-filtered
// target zones, skipping the source zone itself. Replaces the per-kind plural
// wrappers, which now delegate here.
func createProxyRoutes(
	ctx context.Context,
	meshConfig *eventv1.MeshConfig,
	sourceZone *adminv1.Zone,
	targetZones []*adminv1.Zone,
	build proxyRouteBuilder,
	opts ...Option,
) (map[string]*gatewayapi.Route, error) {
	if meshConfig == nil {
		return nil, ctrlerrors.BlockedErrorf("meshConfig must not be nil")
	}

	logger := log.FromContext(ctx)
	routes := map[string]*gatewayapi.Route{}
	zones := collectZones(targetZones, meshConfig.FullMesh, meshConfig.ZoneNames)
	logger.V(1).Info("Collected target zones for proxy Routes", "before", len(targetZones), "after", len(zones))

	for _, targetZone := range zones {
		if ctypes.Equals(sourceZone, targetZone) {
			// ignore the source zone itself if included in the targets (full mesh)
			continue
		}
		route, err := build(ctx, sourceZone, targetZone, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create proxy Route for target zone %q", targetZone.Name)
		}
		routes[targetZone.Name] = route
		logger.V(1).Info("Created proxy Route for target zone", "targetZone", targetZone.Name, "route", ctypes.ObjectRefFromObject(route).String())
	}

	return routes, nil
}

// collectZones filters the given candidate zones based on the mesh configuration.
// If fullMesh is true, all candidates are returned.
func collectZones(candidates []*adminv1.Zone, fullMesh bool, wanted []string) []*adminv1.Zone {
	if fullMesh {
		return candidates
	}

	wantedSet := make(map[string]struct{})
	for _, name := range wanted {
		wantedSet[name] = struct{}{}
	}

	var collected []*adminv1.Zone
	for _, zone := range candidates {
		if _, ok := wantedSet[zone.Name]; ok {
			collected = append(collected, zone)
		}
	}

	return collected
}
