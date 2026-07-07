// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// CreatePublishRoute creates a Route for the publishing events
// The Route is created once per zone where the event-feature is configured
// and points to an internal service
func CreatePublishRoute(
	ctx context.Context,
	zone *adminv1.Zone,
	eventConfig *eventv1.EventConfig,
	opts ...Option,
) (*gatewayv1.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	c := cclient.ClientFromContextOrDie(ctx)
	name := makePublishRouteName(eventConfig)

	// Resolve default preset for hostnames/paths
	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no default preset: %s", zone.Name, err)
	}
	if zone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no gateway reference in status", zone.Name)
	}

	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: zone.Status.Namespace,
		},
	}

	upstream, err := parseUpstream(eventConfig.Spec.PublishEventUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse publishEventUrl %q", eventConfig.Spec.PublishEventUrl)
	}

	// The publish route serves two downstream paths; the events path is first so it becomes the main path.
	hostnames, eventsPaths := preset.ResolveHostnamesAndPaths(makePublishEventsRoutePath())
	_, publishPaths := preset.ResolveHostnamesAndPaths(makePublishRoutePath())
	paths := slices.Concat(eventsPaths, publishPaths)

	mutator := func() error {
		if refErr := controllerutil.SetControllerReference(eventConfig, route, c.Scheme()); refErr != nil {
			return errors.Wrap(refErr, "failed to set controller reference to EventConfig")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): zone.Name,
			config.BuildLabelKey("type"): "publish",
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
		}
		options.applySecurity(route)

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update publish Route %q", ctypes.ObjectRefFromObject(route).String())
	}

	return route, nil
}
