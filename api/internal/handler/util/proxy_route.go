// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateProxyRoute(ctx context.Context, downstreamZoneRef types.ObjectRef, upstreamZoneRef types.ObjectRef, apiBasePath string, realmName string) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// Downstream
	downstreamZone, err := GetZone(ctx, c, downstreamZoneRef.K8s())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get downstream-zone %s", downstreamZoneRef.String())
	}
	downstreamRealmRef := client.ObjectKey{
		Name:      realmName,
		Namespace: downstreamZone.Status.Namespace,
	}
	downstreamRealm, err := GetRealm(ctx, downstreamRealmRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get downstream-realm %s", downstreamRealmRef.String())
	}

	// Upstream
	upstreamZone, err := GetZone(ctx, c, upstreamZoneRef.K8s())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get upstream-zone %s", upstreamZoneRef.String())
	}
	upstreamRealmRef := client.ObjectKey{
		Name:      realmName,
		Namespace: upstreamZone.Status.Namespace,
	}
	upstreamRealm, err := GetRealm(ctx, upstreamRealmRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get upstream-realm %s", upstreamZone.Name)
	}

	routeName := labelutil.NormalizeValue(apiBasePath)
	if realmName != "default" {
		routeName = realmName + "--" + labelutil.NormalizeValue(apiBasePath)
	}

	// Creating the Route
	proxyRoute := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: downstreamZone.Status.Namespace,
		},
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

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, proxyRoute, mutate)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create proxy route")
	}

	return proxyRoute, nil
}
