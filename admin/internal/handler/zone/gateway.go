// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

	var pending []error
	// stopOrCollect returns true when the pipeline must stop: an observed Zone deletion or
	// recreation ends the loop immediately so no later Gateway or Consumer is written.
	stopOrCollect := func(err error) bool {
		if err == nil {
			return false
		}
		if errors.Is(err, errZoneNotLive) {
			return true
		}
		pending = append(pending, err)
		return false
	}
	hc.Zone.Status.Gateways = nil
	for i := range hc.Zone.Spec.Gateways {
		config := &hc.Zone.Spec.Gateways[i]
		gateway, err := createGateway(ctx, hc, config, adminClient)
		if err != nil {
			if stopOrCollect(err) {
				return err
			}
			continue
		}
		consumer, err := createGatewayConsumer(ctx, hc, config.Name, gateway)
		if err != nil {
			if stopOrCollect(err) {
				return err
			}
			continue
		}
		hc.Gateways[config.Name] = gateway
		hc.GatewayConsumers[config.Name] = consumer
		hc.Zone.Status.Gateways = append(hc.Zone.Status.Gateways, adminapi.GatewayStatus{
			Name: config.Name, Types: gatewayTypes(&hc.Zone.Spec, config.Name),
			Gateway: types.ObjectRefFromObject(gateway), AdminClient: types.ObjectRefFromObject(adminClient), Consumer: types.ObjectRefFromObject(consumer),
		})
	}
	if len(pending) > 0 {
		return pending[0]
	}
	return nil
}

// createGateway publishes the Gateway staged against its authoritative read. An invalid configuration
// leaves the published Gateway unchanged.
func createGateway(ctx context.Context, hc *HandlingContext, config *adminapi.GatewayConfig, adminClient *identityapi.Client) (*gatewayapi.Gateway, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	gateway := &gatewayapi.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      naming.ForGateway(hc.Zone, config.Name),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}
	existing := &gatewayapi.Gateway{ObjectMeta: *gateway.ObjectMeta.DeepCopy()}
	exists, err := hc.getExisting(ctx, existing)
	if err != nil {
		return nil, err
	}
	var staged client.Object
	if exists {
		if provErr := checkChildProvenance(hc, existing); provErr != nil {
			return nil, provErr
		}
		staged = existing
	}
	idp, err := hc.Zone.Spec.GetIdentityProviderByName(config.Admin.IdentityProviderRef)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("cannot resolve admin identity provider for gateway %q: %s", config.Name, err)
	}
	hostname, err := issuerHostname(idp)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("cannot resolve issuer hostname for gateway %q: %s", config.Name, err)
	}
	issuerURL := fmt.Sprintf("https://%s/auth/realms/%s", hostname, hc.InternalIdentityRealm.Name)

	if liveErr := hc.Handler.requireLive(ctx, hc.Zone); liveErr != nil {
		return nil, liveErr
	}
	var guardErr error
	mutator := func() error {
		guardErr = stagedGuard(gateway, staged)
		if guardErr != nil {
			return guardErr
		}
		if gateway.Labels == nil {
			gateway.Labels = make(map[string]string)
		}
		gateway.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		gateway.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		gateway.Labels[cconfig.DomainLabelKey] = domainName
		setZoneUID(hc, gateway)

		gateway.Spec = gatewayapi.GatewaySpec{
			Admin: gatewayapi.AdminConfig{
				ClientId:     adminClient.Spec.ClientId,
				ClientSecret: adminClient.Spec.ClientSecret,
				IssuerUrl:    issuerURL,
				Url:          config.Admin.Url,
				TLS:          providerTLS(hc.Trust),
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

	_, err = c.CreateOrUpdate(ctx, gateway, mutator)
	if guardErr != nil {
		return nil, guardErr
	}
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Gateway %s in zone %s: %s", gateway.Name, hc.Zone.Name, err)
	}
	return gateway, nil
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
