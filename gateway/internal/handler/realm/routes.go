// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"fmt"
	"net/url"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type RouteType string

const (
	RouteTypeIssuer    RouteType = "issuer"
	RouteTypeCerts     RouteType = "certs"
	RouteTypeDiscovery RouteType = "discovery"
)

type routeConfig struct {
	UpstreamPathFormat   string
	DownstreamPathFormat string
}

var routeMap = map[RouteType]routeConfig{
	RouteTypeIssuer: {
		UpstreamPathFormat:   "/api/v1/issuer/%s",
		DownstreamPathFormat: "/auth/realms/%s",
	},
	RouteTypeCerts: {
		UpstreamPathFormat:   "/api/v1/certs/%s",
		DownstreamPathFormat: "/auth/realms/%s/protocol/openid-connect/certs",
	},
	RouteTypeDiscovery: {
		UpstreamPathFormat:   "/api/v1/discovery/%s",
		DownstreamPathFormat: "/auth/realms/%s/.well-known/openid-configuration",
	},
}

func CreateRoute(ctx context.Context, realm *gatewayv1.Realm, routeType RouteType) (*gatewayv1.Route, error) {
	c := client.ClientFromContextOrDie(ctx)

	cfg, exists := routeMap[routeType]
	if !exists {
		return nil, errors.Errorf("route type %s not found", routeType)
	}

	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realm.Name + "--" + string(routeType),
			Namespace: realm.Namespace,
		},
	}

	url, err := url.Parse(realm.Spec.Url)
	if err != nil {
		return route, errors.Wrap(err, "failed to parse URL")
	}

	mutator := func() error {
		err := controllerutil.SetControllerReference(realm, route, c.Scheme())
		if err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		route.Spec = gatewayv1.RouteSpec{
			Realm:       *types.ObjectRefFromObject(realm),
			PassThrough: true,
			Upstreams: []gatewayv1.Upstream{
				{
					Scheme: "http",
					Host:   "localhost",
					Port:   8081,
					Path:   fmt.Sprintf(cfg.UpstreamPathFormat, realm.Name),
				},
			},
			Downstreams: []gatewayv1.Downstream{
				{
					Host:      url.Hostname(),
					Port:      gatewayv1.GetPortOrDefaultFromScheme(url),
					Path:      fmt.Sprintf(cfg.DownstreamPathFormat, realm.Name),
					IssuerUrl: "",
				},
			},
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return route, errors.Wrap(err, "failed to create or update route")
	}
	return route, nil
}
