// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/admin/internal/handler/util/urls"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// createGateway creates the Gateway resource for the zone.
func createGateway(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)

	gateway := &gatewayapi.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(naming.ForGateway()),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	mutator := func() error {
		if gateway.Labels == nil {
			gateway.Labels = make(map[string]string)
		}
		gateway.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		gateway.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name

		adminUrl := urls.ForGatewayAdminUrl(hc.Zone.Spec.Gateway.Url)
		if hc.Zone.Spec.Gateway.Admin.Url != "" {
			adminUrl = hc.Zone.Spec.Gateway.Admin.Url
		}

		var clientId, clientSecret, issuerUrl string

		if hc.Zone.Spec.Gateway.Admin.IsExternallyManaged() {
			// Externally managed: use user-provided values with sensible defaults
			clientId = naming.ForGatewayAdminClientId()
			if hc.Zone.Spec.Gateway.Admin.ClientId != nil {
				clientId = *hc.Zone.Spec.Gateway.Admin.ClientId
			}
			if hc.Zone.Spec.Gateway.Admin.ClientSecret != nil {
				clientSecret = *hc.Zone.Spec.Gateway.Admin.ClientSecret
			}
			issuerUrl = urls.ForGatewayAdminIssuerUrl(hc.Zone.Spec.IdentityProvider.Url)
			if hc.Zone.Spec.Gateway.Admin.TokenUrl != nil {
				issuerUrl = *hc.Zone.Spec.Gateway.Admin.TokenUrl
			}
		} else {
			// Managed: use the created rover admin client
			clientId = hc.GatewayAdminClient.Spec.ClientId
			clientSecret = hc.GatewayAdminClient.Spec.ClientSecret
			issuerUrl = urls.ForGatewayAdminIssuerUrl(hc.Zone.Spec.IdentityProvider.Url)
		}

		gateway.Spec = gatewayapi.GatewaySpec{
			Admin: gatewayapi.AdminConfig{
				ClientId:     clientId,
				ClientSecret: clientSecret,
				IssuerUrl:    issuerUrl,
				Url:          adminUrl,
			},
			Redis: gatewayapi.RedisConfig{
				Host:      hc.Zone.Spec.Redis.Host,
				Port:      hc.Zone.Spec.Redis.Port,
				Password:  hc.Zone.Spec.Redis.Password,
				EnableTLS: hc.Zone.Spec.Redis.EnableTLS,
			},
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, gateway, mutator)
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update Gateway %s in zone %s: %s", gateway.Name, hc.Zone.Name, err)
	}

	hc.Gateway = gateway
	hc.Zone.Status.Gateway = types.ObjectRefFromObject(gateway)
	return nil
}

// createDefaultGatewayRealm creates the default gateway realm for the zone.
func createDefaultGatewayRealm(ctx context.Context, hc *HandlingContext) error {
	realm, err := createGatewayRealm(ctx, hc, naming.ForDefaultGatewayRealm(hc.Environment))
	if err != nil {
		return err
	}

	hc.DefaultGatewayRealm = realm
	hc.Zone.Status.GatewayRealm = types.ObjectRefFromObject(realm)
	return nil
}

// createGatewayConsumer creates the default gateway consumer for the zone.
func createGatewayConsumer(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)

	gatewayConsumer := &gatewayapi.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.ForGatewayConsumer(),
			Namespace: hc.Namespace.Name,
		},
	}

	mutator := func() error {
		if gatewayConsumer.Labels == nil {
			gatewayConsumer.Labels = make(map[string]string)
		}
		gatewayConsumer.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		gatewayConsumer.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name

		gatewayConsumer.Spec = gatewayapi.ConsumerSpec{
			Realm: *types.ObjectRefFromObject(hc.DefaultGatewayRealm),
			Name:  naming.ForGatewayConsumer(),
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, gatewayConsumer, mutator)
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update Gateway Consumer %s in zone %s: %s", naming.ForGatewayConsumer(), hc.Zone.Name, err)
	}

	hc.GatewayConsumer = gatewayConsumer
	hc.Zone.Status.GatewayConsumer = types.ObjectRefFromObject(gatewayConsumer)
	return nil
}

// createGatewayRealm is a shared helper that creates a gateway realm with the given name.
func createGatewayRealm(ctx context.Context, hc *HandlingContext, realmName string) (*gatewayapi.Realm, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	gatewayRealm := &gatewayapi.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realmName,
			Namespace: hc.Namespace.Name,
		},
	}

	mutator := func() error {
		if gatewayRealm.Labels == nil {
			gatewayRealm.Labels = make(map[string]string)
		}
		gatewayRealm.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		gatewayRealm.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name

		var routeOverwrites []gatewayapi.RouteOverwrite
		// If the zone is WORLD visible, the gateway is considered a "SpaceGate"
		// to reduce internet-facing exposure the actual IDP routes are not exposed directly
		// but via a proxy route "/auth/realms/<realm>". However, this path is already used for
		// the Gateway Realm itself, so we need to add another prefix to avoid conflicts.
		if hc.Zone.Spec.Visibility == adminv1.ZoneVisibilityWorld {
			for _, rt := range []gatewayapi.RouteType{
				gatewayapi.RouteTypeIssuer,
				gatewayapi.RouteTypeCerts,
				gatewayapi.RouteTypeDiscovery,
			} {
				routeOverwrites = append(routeOverwrites, gatewayapi.RouteOverwrite{
					Type:       rt,
					Enabled:    true,
					PathPrefix: spacegatePathPrefix,
				})
			}
		}

		gatewayRealm.Spec = gatewayapi.RealmSpec{
			Gateway:          types.ObjectRefFromObject(hc.Gateway),
			Urls:             []string{hc.Zone.Spec.Gateway.Url},
			IssuerUrls:       []string{urls.ForGatewayRealm(hc.Zone.Spec.IdentityProvider.Url, realmName)},
			DefaultConsumers: []string{},
			RouteOverwrites:  routeOverwrites,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, gatewayRealm, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Gateway Realm %s in zone %s: %s", realmName, hc.Zone.Name, err)
	}
	return gatewayRealm, nil
}
