// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ handler.Handler[*gatewayv1.Gateway] = &GatewayHandler{}

type GatewayHandler struct{}

func (h *GatewayHandler) CreateOrUpdate(ctx context.Context, gw *gatewayv1.Gateway) error {

	gw.SetCondition(condition.NewDoneProcessingCondition("Created Gateway"))
	gw.SetCondition(condition.NewReadyCondition("Ready", "Gateway is ready"))
	return nil
}

func (h *GatewayHandler) Delete(ctx context.Context, object *gatewayv1.Gateway) error {

	return nil
}
