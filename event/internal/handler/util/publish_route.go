// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// CreatePublishRoute creates a Route for the publishing events
// The Route is created once per zone where the event-feature is configured
// and points to an internal service
func CreatePublishRoute(
	ctx context.Context,
	zone *adminv1.Zone,
	eventConfig *eventv1.EventConfig,
) (*gatewayv1.Route, error) {

	c := cclient.ClientFromContextOrDie(ctx)
	name := makePublishRouteName(eventConfig)

	gatewayRealm := &gatewayv1.Realm{}
	err := c.Get(ctx, zone.Status.GatewayRealm.K8s(), gatewayRealm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("realm %q not found", zone.Status.GatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get realm %q", zone.Status.GatewayRealm.String())
	}
	if err := condition.EnsureReady(gatewayRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", gatewayRealm.Name)
	}

	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: zone.Status.Namespace,
		},
	}

	publishUrl, err := url.Parse(eventConfig.Spec.PublishEventUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse publishEventUrl %q", eventConfig.Spec.PublishEventUrl)
	}

	upstream := gatewayv1.Upstream{
		Scheme: publishUrl.Scheme,
		Host:   publishUrl.Hostname(),
		Path:   publishUrl.Path,
		Port:   gatewayv1.GetPortOrDefaultFromScheme(publishUrl),
	}

	downstream, err := gatewayRealm.AsDownstream(makePublishRoutePath(zone.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create downstream for publish Route")
	}
	mutator := func() error {
		if err := controllerutil.SetControllerReference(eventConfig, route, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference to EventConfig")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:         "event",
			config.BuildLabelKey("zone"):  zone.Name,
			config.BuildLabelKey("realm"): gatewayRealm.Name,
			config.BuildLabelKey("type"):  "publish",
		}

		route.Spec = gatewayv1.RouteSpec{
			Upstreams: []gatewayv1.Upstream{
				upstream,
			},
			Downstreams: []gatewayv1.Downstream{
				downstream,
			},
			Realm: *ctypes.ObjectRefFromObject(gatewayRealm),
			Security: &gatewayv1.Security{
				DisableAccessControl: true,
			},
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update publish Route %q", ctypes.ObjectRefFromObject(route).String())
	}

	return route, nil
}
