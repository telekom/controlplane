// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	aurorv1 "github.com/telekom/controlplane/aurora/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// TelecontextConfig holds configuration for Telecontext auto-integration.
type TelecontextConfig struct {
	// ConsumerName is the consumer name of the Telecontext application.
	ConsumerName string
}

// CreateTelecontextConsumeRoute creates an automatic ConsumeRoute granting
// the Telecontext application access to an MCP route.
// Only called when variant is TELECONTEXTMCP.
func CreateTelecontextConsumeRoute(
	ctx context.Context,
	route *gatewayapi.Route,
	zone *adminv1.Zone,
	telecontextConfig TelecontextConfig,
) (*gatewayapi.ConsumeRoute, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	consumeRouteName := route.Name + "--" + labelutil.NormalizeNameValue(telecontextConfig.ConsumerName)

	consumeRoute := &gatewayapi.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consumeRouteName,
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		consumeRoute.Labels = map[string]string{
			config.DomainLabelKey:          "aurora",
			aurorv1.McpBasePathLabelKey:    route.Labels[aurorv1.McpBasePathLabelKey],
			config.BuildLabelKey("zone"):   zone.Name,
			config.BuildLabelKey("type"):   "telecontext",
		}

		consumeRoute.Spec = gatewayapi.ConsumeRouteSpec{
			Route:        *ctypes.ObjectRefFromObject(route),
			ConsumerName: telecontextConfig.ConsumerName,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, consumeRoute, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update Telecontext ConsumeRoute %s/%s", consumeRoute.Namespace, consumeRoute.Name)
	}

	return consumeRoute, nil
}
