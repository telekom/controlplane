// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/admin/internal/handler/util/urls"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	zoneLabelName = "zone"

	// spacegatePathPrefix is the downstream path prefix added to all identity
	// routes (issuer, certs, discovery) when a zone's visibility is World.
	spacegatePathPrefix = "/spacegate"

	claimOriginZone     = "originZone"
	claimOriginStargate = "originStargate"
	claimClientId       = "clientId"

	EnablePassThrough = true
)

var _ handler.Handler[*adminv1.Zone] = &ZoneHandler{}

// ZoneHandler implements the Handler interface for Zone resources.
type ZoneHandler struct{}

func (h *ZoneHandler) CreateOrUpdate(ctx context.Context, obj *adminv1.Zone) error {
	hc, err := newHandlingContext(ctx, obj)
	if err != nil {
		return err
	}

	steps := []Step{
		createIdentityProvider,
		createDefaultIdentityRealm,
		createInternalIdentityRealm,
		createGatewayAdminClient,
		createGatewayClient,
		createGateway,
		createGatewayConsumer,
		reconcileInternalRoutes,
		createIdentityRoutes,
		cleanupStaleRoutes,
		populateLinks,
	}

	for _, step := range steps {
		if err := step(ctx, hc); err != nil {
			return err
		}
	}

	c := cclient.ClientFromContextOrDie(ctx)

	if c.AnyChanged() {
		obj.SetCondition(condition.NewNotReadyCondition("SubResourceProvisioning", "At least one sub-resource has been created or updated"))
		return nil
	}

	// AI Gateway configuration
	if cconfig.FeatureAiGateway.IsEnabled() && obj.Spec.AiGateway != nil {
		if err := reconcileAiGateway(ctx, hc, obj); err != nil {
			return err
		}
	} else {
		obj.Status.AiGateway = nil
		obj.ManageFeature(adminv1.FeatureAiGateway, false)
	}

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("SubResourceProvisioned", "Waiting for sub-resources to be ready"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("ZoneProvisioned", "Zone has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Zone has been provisioned"))

	return nil
}

func reconcileAiGateway(ctx context.Context, hc *HandlingContext, zone *adminv1.Zone) error {
	aiGateway, err := createAiGateway(ctx, hc)
	if err != nil {
		return err
	}
	zone.Status.AiGateway = types.ObjectRefFromObject(aiGateway)

	zone.EnableFeature(adminv1.FeatureAiGateway)
	return nil
}

//nolint:dupl // intentional similarity with createGateway for separate AI gateway config
func createAiGateway(ctx context.Context, hc *HandlingContext) (*gatewayapi.Gateway, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	gateway := &gatewayapi.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(naming.ForAiGateway()),
			Namespace: labelutil.NormalizeValue(hc.Namespace.Name),
		},
	}

	mutator := func() error {
		if gateway.Labels == nil {
			gateway.Labels = make(map[string]string)
		}
		gateway.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		gateway.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name

		clientId := naming.ForGatewayAdminClientId()
		if hc.Zone.Spec.AiGateway.Admin.ClientId != nil {
			clientId = *hc.Zone.Spec.AiGateway.Admin.ClientId
		}

		clientSecret := ""
		if hc.Zone.Spec.AiGateway.Admin.ClientSecret != nil {
			clientSecret = *hc.Zone.Spec.AiGateway.Admin.ClientSecret
		}

		gateway.Spec = gatewayapi.GatewaySpec{
			Admin: gatewayapi.AdminConfig{
				ClientId:     clientId,
				ClientSecret: clientSecret,
				IssuerUrl:    urls.ForGatewayAdminIssuerUrl(hc.Zone.Spec.IdentityProvider.Url),
				Url:          hc.Zone.Spec.AiGateway.Admin.Url,
			},
		}

		if hc.Zone.Spec.Redis != nil {
			gateway.Spec.Redis = &gatewayapi.RedisConfig{
				Host:      hc.Zone.Spec.Redis.Host,
				Port:      hc.Zone.Spec.Redis.Port,
				Password:  hc.Zone.Spec.Redis.Password,
				EnableTLS: hc.Zone.Spec.Redis.EnableTLS,
			}
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, gateway, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update AI Gateway in zone %s", hc.Zone.Name)
	}
	return gateway, nil
}

func (h *ZoneHandler) Delete(ctx context.Context, obj *adminv1.Zone) error {
	return nil
}
