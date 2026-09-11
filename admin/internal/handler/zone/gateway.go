// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
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

		adminUrl := hc.Zone.Spec.Gateway.Admin.Url

		clientId := hc.GatewayAdminClient.Spec.ClientId
		clientSecret := hc.GatewayAdminClient.Spec.ClientSecret
		issuerUrl := urls.ForGatewayAdminIssuerUrl(hc.Zone.Spec.IdentityProvider.Url)

		gateway.Spec = gatewayapi.GatewaySpec{
			Admin: gatewayapi.AdminConfig{
				ClientId:     clientId,
				ClientSecret: clientSecret,
				IssuerUrl:    issuerUrl,
				Url:          adminUrl,
			},
		}

		if hc.Zone.Spec.Redis != nil {
			gateway.Spec.Redis = &gatewayapi.RedisConfig{
				Host:      hc.Zone.Spec.Redis.Host,
				Port:      hc.Zone.Spec.Redis.Port,
				Password:  hc.Zone.Spec.Redis.Password,
				EnableTLS: hc.Zone.Spec.Redis.EnableTLS,
			}

			hc.Zone.EnableFeature(adminapi.FeatureRateLimiting)
		} else {
			hc.Zone.ManageFeature(adminapi.FeatureRateLimiting, false)
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
			Gateway: *types.ObjectRefFromObject(hc.Gateway),
			Name:    naming.ForGatewayConsumer(),
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
