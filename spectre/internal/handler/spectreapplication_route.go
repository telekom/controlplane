// SPDX-FileCopyrightText: 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

// ensureSSERoute creates or updates the gateway Route for SSE delivery of a Spectre listener.
func (h *SpectreApplicationHandler) ensureSSERoute(
	ctx context.Context,
	zone *adminv1.Zone,
	eventConfig *eventv1.EventConfig,
	appId string,
) (*gatewayv1.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	if zone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no gateway reference in status", zone.Name)
	}

	if !eventConfig.IsLocal() {
		return nil, ctrlerrors.BlockedErrorf("EventConfig %q is not local; SSE Route requires a local zone", eventConfig.Name)
	}

	upstream, err := parseSSEUpstream(eventConfig.Spec.Local.ServerSendEventUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse ServerSendEventUrl %q", eventConfig.Spec.Local.ServerSendEventUrl)
	}

	eventType := util.BuildListenerEventType(appId)
	routePath := makeSpectreSSERoutePath(eventType)

	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no default gateway preset: %s", zone.Name, err)
	}

	hostnames, paths := preset.ResolveHostnamesAndPaths(routePath)

	routeName := makeSpectreSSERouteName(appId)
	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
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
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update SSE Route %q", routeName)
	}

	return route, nil
}

// makeSpectreSSERouteName returns a deterministic Route name for a Spectre listener's SSE endpoint.
func makeSpectreSSERouteName(appId string) string {
	return "spectre-sse--" + labelutil.NormalizeNameValue(appId)
}

// makeSpectreSSERoutePath builds the SSE path for a Spectre listener event type.
func makeSpectreSSERoutePath(eventType string) string {
	return "/sse/v1/" + strings.ToLower(eventType)
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
