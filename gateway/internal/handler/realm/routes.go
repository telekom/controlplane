// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"fmt"
	"net/url"
	"path"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type routeConfig struct {
	UpstreamPathFormat   string
	DownstreamPathFormat string
}

func (c routeConfig) UpstreamPath(realmName string) string {
	return fmt.Sprintf(c.UpstreamPathFormat, realmName)
}

func (c routeConfig) DownstreamPath(realmName string) string {
	return fmt.Sprintf(c.DownstreamPathFormat, realmName)
}

func (c routeConfig) DownstreamPathWithPrefix(realmName, prefix string) string {
	if prefix == "" {
		return c.DownstreamPath(realmName)
	}
	return path.Join(prefix, c.DownstreamPath(realmName))
}

var routeMap = map[gatewayv1.RouteType]routeConfig{
	gatewayv1.RouteTypeIssuer: {
		UpstreamPathFormat:   "/api/v1/issuer/%s",
		DownstreamPathFormat: "/auth/realms/%s",
	},
	gatewayv1.RouteTypeCerts: {
		UpstreamPathFormat:   "/api/v1/certs/%s",
		DownstreamPathFormat: "/auth/realms/%s/protocol/openid-connect/certs",
	},
	gatewayv1.RouteTypeDiscovery: {
		UpstreamPathFormat:   "/api/v1/discovery/%s",
		DownstreamPathFormat: "/auth/realms/%s/.well-known/openid-configuration",
	},
}

func findRouteOverwrite(realm *gatewayv1.Realm, routeType gatewayv1.RouteType) *gatewayv1.RouteOverwrite {
	for i := range realm.Spec.RouteOverwrites {
		if realm.Spec.RouteOverwrites[i].Type == routeType {
			return &realm.Spec.RouteOverwrites[i]
		}
	}
	return nil
}

func CreateRoute(ctx context.Context, realm *gatewayv1.Realm, routeType gatewayv1.RouteType) (*gatewayv1.Route, error) {
	c := client.ClientFromContextOrDie(ctx)

	cfg, exists := routeMap[routeType]
	if !exists {
		return nil, errors.Errorf("route type %s not found", routeType)
	}

	overwrite := findRouteOverwrite(realm, routeType)
	if overwrite != nil && !overwrite.Enabled {
		return nil, nil
	}

	var downstreamPrefix string
	if overwrite != nil {
		downstreamPrefix = overwrite.PathPrefix
	}

	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realm.Name + "--" + string(routeType),
			Namespace: realm.Namespace,
		},
	}

	if len(realm.Spec.Urls) == 0 {
		return route, errors.New("realm has no URLs configured")
	}
	url, err := url.Parse(realm.Spec.Urls[0])
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
					Path:      path.Join(url.Path, cfg.DownstreamPathWithPrefix(realm.Name, downstreamPrefix)),
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
