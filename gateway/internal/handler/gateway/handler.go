// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"
	"fmt"

	cc "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ handler.Handler[*gatewayv1.Gateway] = &GatewayHandler{}

type GatewayHandler struct{}

func (h *GatewayHandler) CreateOrUpdate(ctx context.Context, gw *gatewayv1.Gateway) error {

	gw.SetCondition(condition.NewDoneProcessingCondition("Created Gateway"))
	gw.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Gateway is ready"))
	return nil
}

func (h *GatewayHandler) Delete(ctx context.Context, gw *gatewayv1.Gateway) error {
	kubeClient := cc.ClientFromContextOrDie(ctx)
	routes := &gatewayv1.RouteList{}
	if err := kubeClient.List(ctx, routes,
		client.MatchingFields{"spec.gatewayRef": types.ObjectRefFromObject(gw).String()},
	); err != nil {
		return fmt.Errorf("listing routes for gateway %q: %w", gw.Name, err)
	}
	if len(routes.Items) > 0 {
		return ctrlerrors.BlockedErrorf("gateway %q is still referenced by %d route(s)", gw.Name, len(routes.Items))
	}

	consumers := &gatewayv1.ConsumerList{}
	if err := kubeClient.List(ctx, consumers,
		client.MatchingFields{"spec.gatewayRef": types.ObjectRefFromObject(gw).String()},
	); err != nil {
		return fmt.Errorf("listing consumers for gateway %q: %w", gw.Name, err)
	}
	if len(consumers.Items) > 0 {
		return ctrlerrors.BlockedErrorf("gateway %q is still referenced by %d consumer(s)", gw.Name, len(consumers.Items))
	}

	return nil
}
