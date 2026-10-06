// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/routeinputs"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

const (
	zoneHealthRouteSuffix  = adminv1.ZoneHealthRouteName
	zoneHealthPath         = routeinputs.ZoneHealthPath
	zoneHealthUpstreamPath = "/api/v1/zone-health"
)

func createZoneHealthRoutes(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)
	for _, cfg := range hc.Zone.Spec.Gateways {
		gateway := hc.Gateways[cfg.Name]
		if gateway == nil {
			return ctrlerrors.BlockedErrorf("cannot resolve gateway %q", cfg.Name)
		}
		hostnames, basePaths := routeinputs.HostnamesAndBasePaths(&hc.Zone.Spec, cfg.Name)
		route := &gatewayapi.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      labelutil.NormalizeNameValue(gateway.Name + "--" + zoneHealthRouteSuffix),
				Namespace: hc.Namespace.Name,
			},
		}
		_, err := c.CreateOrUpdate(ctx, route, func() error {
			if route.Labels == nil {
				route.Labels = make(map[string]string)
			}
			route.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
			route.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
			route.Labels[cconfig.DomainLabelKey] = domainName
			route.Labels[cconfig.OwnerUidLabelKey] = string(hc.Zone.UID)

			route.Spec = gatewayapi.RouteSpec{
				GatewayRef: *types.ObjectRefFromObject(gateway),
				Type:       gatewayapi.RouteTypePrimary,
				// Kong requires a service, but healthProbe prevents upstream calls.
				Backend: gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{{
					Scheme: "http", Hostname: "localhost", Port: jumperIdentityPort,
					Path: zoneHealthUpstreamPath,
				}}},
				Hostnames:   hostnames,
				Paths:       routeinputs.Paths(basePaths, zoneHealthPath),
				PassThrough: true,
				Traffic:     gatewayapi.Traffic{HealthProbe: true},
				Security:    gatewayapi.Security{RealmName: hc.Zone.Status.RealmName},
			}
			return nil
		})
		if err != nil {
			return ctrlerrors.RetryableErrorf("failed to create or update zone-health route %s: %s", route.Name, err)
		}
	}
	return nil
}
