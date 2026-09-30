// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"net/http"
	"path"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

const (
	zoneHealthRouteSuffix = "zone-health"
	zoneHealthPath        = "/zone-health"
	// zoneHealthUpstreamPath is a placeholder: the gateway terminates the request
	// itself, so the upstream is never called. Kong still requires a service.
	zoneHealthUpstreamPath = "/api/v1/zone-health"

	// Limits of gatewayapi.RouteSpec.Hostnames and gatewayapi.RouteSpec.Paths.
	maxRouteHostnames = 20
	maxRoutePaths     = 10
)

// createZoneHealthRoutes creates a HEAD-only /zone-health route on every gateway
// of the zone. Cross-zone callers probe it to decide whether the zone is healthy;
// the gateway answers with 200 without calling an upstream.
func createZoneHealthRoutes(ctx context.Context, hc *HandlingContext) error {
	for i := range hc.Zone.Spec.Gateways {
		gatewayName := hc.Zone.Spec.Gateways[i].Name
		gateway := hc.Gateways[gatewayName]
		if gateway == nil {
			return ctrlerrors.BlockedErrorf("cannot resolve gateway %q", gatewayName)
		}
		hostnames, basePaths := gatewayHostnamesAndBasePaths(&hc.Zone.Spec, gatewayName)
		if len(hostnames) == 0 {
			return ctrlerrors.BlockedErrorf("gateway %q has no preset hostnames", gatewayName)
		}
		if len(hostnames) > maxRouteHostnames || len(basePaths) > maxRoutePaths {
			return ctrlerrors.BlockedErrorf("gateway %q has %d hostnames and %d base paths, but a route allows at most %d and %d",
				gatewayName, len(hostnames), len(basePaths), maxRouteHostnames, maxRoutePaths)
		}
		if err := createZoneHealthRoute(ctx, hc, gateway, hostnames, basePaths); err != nil {
			return err
		}
	}
	return nil
}

func createZoneHealthRoute(ctx context.Context, hc *HandlingContext, gateway *gatewayapi.Gateway, hostnames, basePaths []string) error {
	c := cclient.ClientFromContextOrDie(ctx)

	routeName := labelutil.NormalizeNameValue(gateway.Name + "--" + zoneHealthRouteSuffix)
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: hc.Namespace.Name,
		},
	}

	mutator := func() error {
		if route.Labels == nil {
			route.Labels = make(map[string]string)
		}
		route.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		route.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		route.Labels[cconfig.DomainLabelKey] = domainName
		route.Labels[cconfig.OwnerUidLabelKey] = string(hc.Zone.GetUID())

		paths := make([]string, 0, len(basePaths))
		for _, basePath := range basePaths {
			paths = append(paths, path.Join(basePath, zoneHealthPath))
		}

		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *types.ObjectRefFromObject(gateway),
			Type:       gatewayapi.RouteTypePrimary,
			Backend: gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{{
				Scheme:   "http",
				Hostname: "localhost",
				Port:     jumperIdentityPort,
				Path:     zoneHealthUpstreamPath,
			}}},
			Hostnames:          hostnames,
			Paths:              paths,
			Methods:            []string{http.MethodHead},
			RequestTermination: &gatewayapi.RequestTermination{StatusCode: http.StatusOK},
			PassThrough:        true,
			Security:           gatewayapi.Security{RealmName: hc.Zone.Status.RealmName},
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, route, mutator); err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update zone-health route %s in zone %s: %s", routeName, hc.Zone.Name, err)
	}
	return nil
}
