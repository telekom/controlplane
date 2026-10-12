// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
)

var _ handler.Handler[*gatewayv1.Gateway] = &GatewayHandler{}

// TrustValidator resolves the Gateway's CA references without building clients.
type TrustValidator interface {
	Validate(ctx context.Context, gw *gatewayv1.Gateway) error
}

type GatewayHandler struct {
	// Trust validates CA references; nil skips trust validation.
	Trust TrustValidator
}

func (h *GatewayHandler) CreateOrUpdate(ctx context.Context, gw *gatewayv1.Gateway) error {
	if h.Trust != nil {
		err := h.Trust.Validate(ctx, gw)
		SetTrustCondition(gw, err)
		if kongutil.AsTrustError(err) != nil {
			// The Gateway stays usable as configuration holder; dependents block on acquisition.
			log := logr.FromContextOrDiscard(ctx)
			log.Info("Gateway trust is not ready", "reason", err.Error())
		} else if err != nil {
			return fmt.Errorf("validating gateway trust: %w", err)
		}
	}
	gw.SetCondition(condition.NewDoneProcessingCondition("Created Gateway"))
	gw.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Gateway is ready"))
	return nil
}

func (h *GatewayHandler) Delete(ctx context.Context, object *gatewayv1.Gateway) error {
	return nil
}
