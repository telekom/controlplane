// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
)

func reconcileGateways(ctx context.Context, hc *HandlingContext) error {
	adminClient, err := ensureRoverClient(ctx, hc)
	if err != nil {
		return err
	}
	hc.AdminClient = adminClient

	hc.Zone.Status.Gateways = nil
	for i := range hc.Zone.Spec.Gateways {
		config := &hc.Zone.Spec.Gateways[i]
		if _, err := hc.Zone.Spec.GetIdentityProviderByName(config.Admin.IdentityProviderRef); err != nil {
			return ctrlerrors.BlockedErrorf("cannot resolve admin identity provider for gateway %q: %s", config.Name, err)
		}
		gateway, err := createGateway(ctx, hc, config, adminClient)
		if err != nil {
			return err
		}
		consumer, err := createGatewayConsumer(ctx, hc, config.Name, gateway)
		if err != nil {
			return err
		}
		hc.Gateways[config.Name] = gateway
		hc.GatewayConsumers[config.Name] = consumer
		hc.Zone.Status.Gateways = append(hc.Zone.Status.Gateways, adminapi.GatewayStatus{
			Name: config.Name, Types: gatewayTypes(&hc.Zone.Spec, config.Name),
			Gateway: types.ObjectRefFromObject(gateway), AdminClient: types.ObjectRefFromObject(adminClient), Consumer: types.ObjectRefFromObject(consumer),
		})
	}
	return nil
}

func createGateway(ctx context.Context, hc *HandlingContext, config *adminapi.GatewayConfig, adminClient *identityapi.Client) (*gatewayapi.Gateway, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	gateway := &gatewayapi.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.ForGateway(hc.Zone, config.Name),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	issuerURL, err := internalIssuerURL(hc)
	if err != nil {
		return nil, err
	}

	mutator := func() error {
		if gateway.Labels == nil {
			gateway.Labels = make(map[string]string)
		}
		gateway.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		gateway.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		gateway.Labels[cconfig.DomainLabelKey] = domainName

		gateway.Spec = gatewayapi.GatewaySpec{
			Admin: gatewayapi.AdminConfig{
				ClientId:     adminClient.Spec.ClientId,
				ClientSecret: adminClient.Spec.ClientSecret,
				IssuerUrl:    issuerURL,
				Url:          config.Admin.Url,
			},
			NamePrefix: gatewayNamePrefix(gateway.Spec.NamePrefix, hc.Zone.Status.RealmName),
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

	_, err = c.CreateOrUpdate(ctx, gateway, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Gateway %s in zone %s: %s", gateway.Name, hc.Zone.Name, err)
	}
	return gateway, nil
}

// gatewayNamePrefix returns the Kong name prefix of a Gateway: the zone's realm name, or the
// current prefix if the realm name is empty, because a set prefix cannot be removed.
func gatewayNamePrefix(current, realmName string) string {
	if realmName == "" {
		return current
	}
	return realmName
}

// createGatewayConsumer creates the default gateway consumer for the zone.
func createGatewayConsumer(ctx context.Context, hc *HandlingContext, gatewayName string, gateway *gatewayapi.Gateway) (*gatewayapi.Consumer, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	gatewayConsumer := &gatewayapi.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.ForGatewayConsumer(hc.Zone, gatewayName),
			Namespace: hc.Namespace.Name,
		},
	}

	mutator := func() error {
		if gatewayConsumer.Labels == nil {
			gatewayConsumer.Labels = make(map[string]string)
		}
		gatewayConsumer.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		gatewayConsumer.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		gatewayConsumer.Labels[cconfig.DomainLabelKey] = domainName

		gatewayConsumer.Spec = gatewayapi.ConsumerSpec{
			Gateway: *types.ObjectRefFromObject(gateway),
			Name:    naming.ForGatewayConsumerClientID(),
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, gatewayConsumer, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Gateway Consumer %s in zone %s: %s", naming.ForGatewayConsumer(hc.Zone, gatewayName), hc.Zone.Name, err)
	}
	return gatewayConsumer, nil
}

// gatewayTypes returns the traffic types a gateway serves, derived from the presets
// referencing it. Sorted for a canonical status order independent of how presets are
// written in the spec.
func gatewayTypes(spec *adminapi.ZoneSpec, gatewayName string) []adminapi.GatewayType {
	var gwTypes []adminapi.GatewayType
	for i := range spec.Presets {
		preset := &spec.Presets[i]
		if preset.GatewayRef == gatewayName && !slices.Contains(gwTypes, preset.Type) {
			gwTypes = append(gwTypes, preset.Type)
		}
	}
	slices.Sort(gwTypes)
	return gwTypes
}
