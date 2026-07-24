// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
)

var _ handler.Handler[*gatewayv1.Gateway] = &GatewayHandler{}

type GatewayHandler struct {
	xdsClient   envoy.XdsClient
	assignments AssignmentRegistry
}

type AssignmentRegistry interface {
	Assign(string, string) error
	RemoveNode(string)
}

func NewGatewayHandler(xdsClient envoy.XdsClient, assignments ...AssignmentRegistry) *GatewayHandler {
	h := &GatewayHandler{xdsClient: xdsClient}
	if len(assignments) > 0 {
		h.assignments = assignments[0]
	}
	return h
}

func (h *GatewayHandler) CreateOrUpdate(ctx context.Context, gw *gatewayv1.Gateway) error {
	if gw.Spec.Type == gatewayv1.GatewayTypeEnvoy {
		nodeID, err := envoy.NodeIDForGateway(gw)
		if err != nil {
			return fmt.Errorf("deriving xDS node ID: %w", err)
		}
		if gw.Spec.RelayIdentity == "" {
			return fmt.Errorf("relay identity is required for Envoy Gateway")
		}
		gw.Status.XDSNodeID = nodeID
		gw.Status.RelayIdentity = gw.Spec.RelayIdentity
		if h.assignments != nil {
			if err := h.assignments.Assign(gw.Spec.RelayIdentity, nodeID); err != nil {
				return fmt.Errorf("installing relay assignment: %w", err)
			}
		}
	} else if gw.Status.XDSNodeID != "" {
		if h.xdsClient != nil {
			if err := h.xdsClient.ClearGateway(ctx, gw); err != nil {
				return fmt.Errorf("clearing previous Envoy snapshot: %w", err)
			}
		}
		if h.assignments != nil {
			h.assignments.RemoveNode(gw.Status.XDSNodeID)
		}
		gw.Status.XDSNodeID = ""
		gw.Status.RelayIdentity = ""
	}

	gw.SetCondition(condition.NewDoneProcessingCondition("Created Gateway"))
	gw.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Gateway is ready"))
	return nil
}

func (h *GatewayHandler) Delete(ctx context.Context, object *gatewayv1.Gateway) error {
	if object.Spec.Type == gatewayv1.GatewayTypeEnvoy && h.xdsClient != nil {
		nodeID, err := envoy.NodeIDForGateway(object)
		if err != nil {
			return err
		}
		if err := h.xdsClient.ClearGateway(ctx, object); err != nil {
			return err
		}
		if h.assignments != nil {
			h.assignments.RemoveNode(nodeID)
		}
	}
	return nil
}
