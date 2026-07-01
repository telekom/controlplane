// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
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

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("SubResourceProvisioned", "Waiting for sub-resources to be ready"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("ZoneProvisioned", "Zone has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Zone has been provisioned"))

	return nil
}

func (h *ZoneHandler) Delete(ctx context.Context, obj *adminv1.Zone) error {
	return nil
}
